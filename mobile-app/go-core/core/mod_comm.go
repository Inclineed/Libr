package core

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/libr-forum/Libr/core/crypto/cryptoutils"
	"github.com/libr-forum/Libr/core/mod_client/network"
	"github.com/libr-forum/Libr/core/mod_client/types"
	util "github.com/libr-forum/Libr/core/mod_client/util"
)

// AutoSendToMods sends a message to all online moderators and collects their
// signatures. Returns the list of valid ModCerts received.
func AutoSendToMods(message string, ts int64) ([]types.ModCert, error) {
	msg := types.Msg{
		Content: message,
		Ts:      ts,
	}

	onlineMods, err := util.GetOnlineMods()
	if err != nil {
		return nil, fmt.Errorf("failed to get online mods: %w", err)
	}
	if len(onlineMods) == 0 {
		return nil, fmt.Errorf("no online moderators available")
	}

	var (
		modcertList []types.ModCert
		mu          sync.Mutex
		wg          sync.WaitGroup
		once        sync.Once
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, mod := range onlineMods {
		wg.Add(1)
		go func(mod types.Mod) {
			defer wg.Done()

			modCtx, modCancel := context.WithTimeout(ctx, 5*time.Second)
			defer modCancel()

			responseChan := make(chan types.ModCert, 1)

			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[auto-mod] panic contacting %s: %v", mod.PeerId, r)
					}
				}()
				response, err := network.SendTo(mod.PeerId, "/route=auto", msg, "mod")
				if err != nil {
					log.Printf("[auto-mod] Failed to contact %s: %v", mod.PeerId, err)
					return
				}
				modcert, ok := response.(types.ModCert)
				if !ok {
					return
				}
				responseChan <- modcert
			}()

			select {
			case <-modCtx.Done():
				log.Printf("[auto-mod] Mod %s timed out", mod.PeerId)
			case modcert := <-responseChan:
				msgHash := msg.Content + strconv.FormatInt(msg.Ts, 10) + modcert.Status
				if cryptoutils.VerifySignature(modcert.PublicKey, msgHash, modcert.Sign) {
					mu.Lock()
					modcertList = append(modcertList, modcert)
					if modcert.Status == "1" {
						// Early-exit once we have a majority approval
						once.Do(func() {
							if len(modcertList) >= (len(onlineMods)/2 + 1) {
								cancel()
							}
						})
					}
					mu.Unlock()
				} else {
					log.Printf("[auto-mod] Invalid signature from %s", mod.PeerId)
				}
			}
		}(mod)
	}

	wg.Wait()
	return modcertList, nil
}

// SendToMods sends a MsgCert to a specific list of mods (used for manual mod).
func SendToMods(cert types.MsgCert, mods []types.Mod) []types.ModCert {
	var (
		modcertList []types.ModCert
		mu          sync.Mutex
		wg          sync.WaitGroup
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, mod := range mods {
		wg.Add(1)
		go func(mod types.Mod) {
			defer wg.Done()

			modCtx, modCancel := context.WithTimeout(ctx, 3*time.Second)
			defer modCancel()

			respChan := make(chan types.ModCert, 1)

			go func() {
				resp, err := network.SendTo(mod.PeerId, "/route=manual", cert, "mod")
				if err != nil {
					return
				}
				modcert, ok := resp.(types.ModCert)
				if ok {
					respChan <- modcert
				}
			}()

			select {
			case <-modCtx.Done():
				log.Printf("[manual-mod] Mod %s unresponsive", mod.PeerId)
			case modcert := <-respChan:
				// Verify: manual_mod uses content+ts+status; others use sign+status
				var msgHash string
				if cert.Type == "manual_mod" {
					msgHash = cert.Msg.Content + strconv.FormatInt(cert.Msg.Ts, 10) + modcert.Status
				} else {
					msgHash = cert.Sign + modcert.Status
				}
				if cryptoutils.VerifySignature(modcert.PublicKey, msgHash, modcert.Sign) {
					mu.Lock()
					modcertList = append(modcertList, modcert)
					mu.Unlock()
				}
			}
		}(mod)
	}

	wg.Wait()
	return modcertList
}
