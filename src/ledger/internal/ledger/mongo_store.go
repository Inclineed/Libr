package ledger

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ---------------------------------------------------------------------------
// MongoStore — implements RelayStore, NodeStore, ModStore, Staler
// ---------------------------------------------------------------------------

// MongoStore persists all ledger data in MongoDB, mirroring the schema used
// by the legacy JS relay server (database "Addrs").
//
// Collections:
//
//	relays     – { peer_id, ws_address, public_key, last_updated } permanent
//	nodes      – { node_id, peer_id, public_key, last_seen }       TTL-based
//	onlinemods – { peer_id, public_key, last_seen }                 TTL-based
//	mods       – { public_key }                                     allowlist (admin-managed)
type MongoStore struct {
	client     *mongo.Client
	db         *mongo.Database
	relays     *mongo.Collection
	nodes      *mongo.Collection
	onlineMods *mongo.Collection
	modAllow   *mongo.Collection
}

// NewMongoStore connects to MongoDB and returns a MongoStore.
// uri is a standard MongoDB connection string.
func NewMongoStore(ctx context.Context, uri string) (*MongoStore, error) {
	clientOpts := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		return nil, fmt.Errorf("mongo connect: %w", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("mongo ping: %w", err)
	}

	db := client.Database("Addrs")
	s := &MongoStore{
		client:     client,
		db:         db,
		relays:     db.Collection("relays"),
		nodes:      db.Collection("nodes"),
		onlineMods: db.Collection("onlinemods"),
		modAllow:   db.Collection("mods"),
	}

	log.Println("MongoDB connected")
	return s, nil
}

// EnsureIndexes creates the necessary indexes.  Call once at startup.
// If nodeTTL / modTTL > 0 the corresponding MongoDB TTL index is created
// (documents are deleted server-side after that duration); the application-
// level expiry job provides a second line of defence.
func (s *MongoStore) EnsureIndexes(ctx context.Context, nodeTTL, modTTL time.Duration) error {
	type indexSpec struct {
		coll    *mongo.Collection
		model   mongo.IndexModel
		purpose string
	}

	uniqueTrue := true
	specs := []indexSpec{
		{
			coll:    s.relays,
			purpose: "relays unique peer_id",
			model: mongo.IndexModel{
				Keys:    bson.D{{Key: "peer_id", Value: 1}},
				Options: &options.IndexOptions{Unique: &uniqueTrue},
			},
		},
		{
			coll:    s.nodes,
			purpose: "nodes unique public_key",
			model: mongo.IndexModel{
				Keys:    bson.D{{Key: "public_key", Value: 1}},
				Options: &options.IndexOptions{Unique: &uniqueTrue},
			},
		},
		{
			coll:    s.onlineMods,
			purpose: "onlinemods unique public_key",
			model: mongo.IndexModel{
				Keys:    bson.D{{Key: "public_key", Value: 1}},
				Options: &options.IndexOptions{Unique: &uniqueTrue},
			},
		},
		{
			coll:    s.modAllow,
			purpose: "mods allowlist public_key",
			model: mongo.IndexModel{
				Keys:    bson.D{{Key: "public_key", Value: 1}},
				Options: &options.IndexOptions{Unique: &uniqueTrue},
			},
		},
	}

	// Optional server-side TTL indexes
	if nodeTTL > 0 {
		secs := int32(nodeTTL.Seconds())
		specs = append(specs, indexSpec{
			coll:    s.nodes,
			purpose: "nodes TTL on last_seen",
			model: mongo.IndexModel{
				Keys:    bson.D{{Key: "last_seen", Value: 1}},
				Options: &options.IndexOptions{ExpireAfterSeconds: &secs},
			},
		})
	}
	if modTTL > 0 {
		secs := int32(modTTL.Seconds())
		specs = append(specs, indexSpec{
			coll:    s.onlineMods,
			purpose: "onlinemods TTL on last_seen",
			model: mongo.IndexModel{
				Keys:    bson.D{{Key: "last_seen", Value: 1}},
				Options: &options.IndexOptions{ExpireAfterSeconds: &secs},
			},
		})
	}

	for _, spec := range specs {
		if _, err := spec.coll.Indexes().CreateOne(ctx, spec.model); err != nil {
			return fmt.Errorf("create index %s: %w", spec.purpose, err)
		}
		log.Printf("index ensured: %s", spec.purpose)
	}
	return nil
}

// Close disconnects the MongoDB client gracefully.
func (s *MongoStore) Close(ctx context.Context) error {
	return s.client.Disconnect(ctx)
}

// StartCleanup starts a background goroutine that runs the application-level
// expiry sweep every interval.  It stops when ctx is cancelled.
func (s *MongoStore) StartCleanup(ctx context.Context, interval time.Duration, entryTTL time.Duration) {
	if entryTTL <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cutoff := time.Now().UTC().Add(-entryTTL)
				staleN, err := s.RemoveStaleNodes(cutoff)
				if err != nil {
					log.Printf("mongo cleanup nodes error: %v", err)
				}
				staleM, err := s.RemoveStaleMods(cutoff)
				if err != nil {
					log.Printf("mongo cleanup mods error: %v", err)
				}
				if staleN+staleM > 0 {
					log.Printf("mongo cleanup removed stale entries nodes=%d mods=%d", staleN, staleM)
				}
			}
		}
	}()
}

// ---------------------------------------------------------------------------
// RelayStore impl
// ---------------------------------------------------------------------------

func (s *MongoStore) Upsert(relay RelayInfo) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	relay.LastUpdated = time.Now().UTC()
	filter := bson.M{"peer_id": relay.PeerID}
	update := bson.M{"$set": relay}
	opts := options.Update().SetUpsert(true)

	_, err := s.relays.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("mongo upsert relay: %w", err)
	}
	log.Printf("relay upserted peer_id=%s", relay.PeerID)
	return nil
}

func (s *MongoStore) GetAll() ([]RelayInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cur, err := s.relays.Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{"_id": 0}))
	if err != nil {
		return nil, fmt.Errorf("mongo find relays: %w", err)
	}
	defer cur.Close(ctx)

	var result []RelayInfo
	if err := cur.All(ctx, &result); err != nil {
		return nil, fmt.Errorf("mongo decode relays: %w", err)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// NodeStore impl
// ---------------------------------------------------------------------------

func (s *MongoStore) UpsertNode(node NodeInfo) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	node.LastSeen = time.Now().UTC()
	filter := bson.M{"peer_id": node.PeerID}
	update := bson.M{"$set": node}
	opts := options.Update().SetUpsert(true)

	_, err := s.nodes.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("mongo upsert node: %w", err)
	}
	log.Printf("node upserted peer_id=%s node_id=%s", node.PeerID, node.NodeID)
	return nil
}

func (s *MongoStore) TouchNode(publicKey string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := s.nodes.UpdateOne(ctx,
		bson.M{"public_key": publicKey},
		bson.M{"$set": bson.M{"last_seen": time.Now().UTC()}},
	)
	if err != nil {
		return false, fmt.Errorf("mongo touch node: %w", err)
	}
	return res.MatchedCount > 0, nil
}

func (s *MongoStore) RemoveNode(publicKey string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.nodes.DeleteOne(ctx, bson.M{"public_key": publicKey})
	if err != nil {
		return fmt.Errorf("mongo remove node: %w", err)
	}
	return nil
}

func (s *MongoStore) GetNodes(ttl time.Duration) ([]NodeInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{}
	if ttl > 0 {
		filter = bson.M{"last_seen": bson.M{"$gte": time.Now().UTC().Add(-ttl)}}
	}

	cur, err := s.nodes.Find(ctx, filter, options.Find().SetProjection(bson.M{"_id": 0}))
	if err != nil {
		return nil, fmt.Errorf("mongo find nodes: %w", err)
	}
	defer cur.Close(ctx)

	var result []NodeInfo
	if err := cur.All(ctx, &result); err != nil {
		return nil, fmt.Errorf("mongo decode nodes: %w", err)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// ModStore impl
// ---------------------------------------------------------------------------

func (s *MongoStore) IsModAllowed(publicKey string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res := s.modAllow.FindOne(ctx, bson.M{"public_key": publicKey},
		options.FindOne().SetProjection(bson.M{"_id": 1}))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			return false, nil
		}
		return false, fmt.Errorf("mongo is mod allowed: %w", err)
	}
	return true, nil
}

func (s *MongoStore) UpsertMod(mod ModInfo) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mod.LastSeen = time.Now().UTC()
	filter := bson.M{"public_key": mod.PublicKey}
	update := bson.M{"$set": mod}
	opts := options.Update().SetUpsert(true)

	_, err := s.onlineMods.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("mongo upsert mod: %w", err)
	}
	log.Printf("mod upserted peer_id=%s public_key=%s", mod.PeerID, mod.PublicKey)
	return nil
}

func (s *MongoStore) TouchMod(publicKey string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := s.onlineMods.UpdateOne(ctx,
		bson.M{"public_key": publicKey},
		bson.M{"$set": bson.M{"last_seen": time.Now().UTC()}},
	)
	if err != nil {
		return false, fmt.Errorf("mongo touch mod: %w", err)
	}
	return res.MatchedCount > 0, nil
}

func (s *MongoStore) RemoveMod(publicKey string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.onlineMods.DeleteOne(ctx, bson.M{"public_key": publicKey})
	if err != nil {
		return fmt.Errorf("mongo remove mod: %w", err)
	}
	return nil
}

func (s *MongoStore) GetMods(ttl time.Duration) ([]ModInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{}
	if ttl > 0 {
		filter = bson.M{"last_seen": bson.M{"$gte": time.Now().UTC().Add(-ttl)}}
	}

	cur, err := s.onlineMods.Find(ctx, filter, options.Find().SetProjection(bson.M{"_id": 0}))
	if err != nil {
		return nil, fmt.Errorf("mongo find mods: %w", err)
	}
	defer cur.Close(ctx)

	var result []ModInfo
	if err := cur.All(ctx, &result); err != nil {
		return nil, fmt.Errorf("mongo decode mods: %w", err)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Staler impl
// ---------------------------------------------------------------------------

func (s *MongoStore) RemoveStaleNodes(cutoff time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := s.nodes.DeleteMany(ctx, bson.M{"last_seen": bson.M{"$lt": cutoff}})
	if err != nil {
		return 0, fmt.Errorf("mongo remove stale nodes: %w", err)
	}
	return int(res.DeletedCount), nil
}

func (s *MongoStore) RemoveStaleMods(cutoff time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := s.onlineMods.DeleteMany(ctx, bson.M{"last_seen": bson.M{"$lt": cutoff}})
	if err != nil {
		return 0, fmt.Errorf("mongo remove stale mods: %w", err)
	}
	return int(res.DeletedCount), nil
}
