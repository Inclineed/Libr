package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	cache "github.com/libr-forum/Libr/core/mod_client/cache_handler"
	moddb "github.com/libr-forum/Libr/core/mod_client/internal/mod_db"
	"github.com/libr-forum/Libr/core/mod_client/logger"
	"github.com/libr-forum/Libr/core/mod_client/models"
	"github.com/libr-forum/Libr/core/mod_client/types"
)

var forbidden = LoadForbiddenWords()

type ModelFunc func(content string) (bool, error)

func init() {
	ensureModConfigExists()
}

func GetModConfigPath() string {
	var path string

	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		path = filepath.Join(appData, "libr", "modconfig", "modconfig.json")
	case "darwin":
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, "Library", "Application Support", "libr", "modconfig", "modconfig.json")
	case "linux":
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".config", "libr", "modconfig", "modconfig.json")
	default:
		path = filepath.Join("modconfig", "modconfig.json")
	}

	return path
}

func GetModKeysPath() string {
	var path string

	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		path = filepath.Join(appData, "libr", "modconfig", "modkeys.json")
	case "darwin":
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, "Library", "Application Support", "libr", "modconfig", "modkeys.json")
	case "linux":
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".config", "libr", "modconfig", "modkeys.json")
	default:
		path = filepath.Join("modconfig", "modkeys.json")
	}

	return path
}

func GetGoogleApiKey() (string, error) {
	path := GetModKeysPath()
	content, err := os.ReadFile(path)
	if err != nil {
		logger.LogToFile("Error reading modkeys")
		return "", fmt.Errorf("failed to read modkeys.json: %w", err)
	}

	var data map[string]string
	if err := json.Unmarshal(content, &data); err != nil {
		logger.LogToFile("failed to parse modkeys")
		return "", fmt.Errorf("failed to parse modkeys.json: %w", err)
	}

	key, ok := data["GOOGLE_NLP_API_KEY"]
	if !ok || key == "" {
		logger.LogToFile("[DEBUG]GOOGLE_NLP_API_KEY not found in modkeys.json")
		return "", fmt.Errorf("GOOGLE_NLP_API_KEY not found in modkeys.json")
	}

	return key, nil
}

func ensureModConfigExists() {
	path := GetModConfigPath()
	defaultJSON := `{
  "forbidden": [],
  "thresholds": "Toxic:0.30,Insult:0.90,Profanity:0.60,Derogatory:0.60,Sexual:0.30,Violence:0.40,Drugs:0.60,Death/Harm/Tragedy:0.60,Firearms/Weapons:0.60,Public Safety:0.30,Health:0.50,Religion/Belief:0.30,War/Conflict:0.70,Politics:0.80,Finance:0.40,Legal:0.60",
  "image_auto_mod": true
}`

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			fmt.Println("Failed to create modconfig directory:", err)
			logger.LogToFile("[DEBUG]Failed to create modconfig directory")
			return
		}
		if err := os.WriteFile(path, []byte(defaultJSON), 0644); err != nil {
			fmt.Println("Failed to write default config:", err)
			logger.LogToFile("[DEBUG]Failed to write default config")
		}
	}
}

func ReadModConfigFile() (models.ModConfig, error) {
	path := GetModConfigPath()
	file, err := os.Open(path)
	if err != nil {
		logger.LogToFile("failed to open modconfig.json")
		return models.ModConfig{}, fmt.Errorf("failed to open modconfig.json: %w", err)
	}
	defer file.Close()

	var config models.ModConfig
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		logger.LogToFile("[DEBUG]Failed to decode modconfig.json")
		return models.ModConfig{}, fmt.Errorf("failed to decode modconfig.json: %w", err)
	}

	return config, nil
}

func LoadForbiddenWords() []string {
	config, err := ReadModConfigFile()
	if err != nil {

		fmt.Println("Error loading forbidden words:", err)
		return nil
	}
	return config.Forbidden
}

var urlRegex = regexp.MustCompile(`(?i)\b((?:https?://|www\d{0,3}[.]|[a-z0-9.\-]+[.][a-z]{2,4}/)[^\s]+)`)

var base64ImgRegex = regexp.MustCompile(`(?i)<img[^>]+src=["'](data:image/[a-zA-Z]*;base64,[^"']+)["'][^>]*>`)

func extractBase64Images(content string) (string, []string) {
	var images []string

	// Find all base64 data URIs
	matches := base64ImgRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 {
			// Extract just the base64 payload part if needed,
			// though Cloud Vision accepts the raw base64 string without data:image prefix sometimes.
			// We will strip the prefix for Cloud Vision later.
			images = append(images, match[1])
		}
	}

	// Remove the entire img tags from the content
	cleanedContent := base64ImgRegex.ReplaceAllString(content, "[IMAGE_REMOVED]")
	return cleanedContent, images
}

func AutoModerateMsg(msg models.UserMsg) (string, error) {
	for _, word := range forbidden {
		if strings.Contains(
			strings.ToLower(msg.Content),
			strings.ToLower(word),
		) {
			return "0", nil
		}
	}

	if urlRegex.MatchString(msg.Content) {
		return "0", nil
	}

	cleanedContent, images := extractBase64Images(msg.Content)
	fmt.Printf("[DEBUG] Extracted %d images from message\n", len(images))

	if len(images) > 0 {
		config, err := ReadModConfigFile()
		if err != nil {
			fmt.Println("Error reading mod config for image automod check:", err)
		} else {
			// Check if ImageAutoMod is enabled (defaults to true if nil)
			if config.ImageAutoMod != nil && !*config.ImageAutoMod {
				fmt.Println("[INFO] ImageAutoMod disabled. Rejecting image.")
				return "0", nil
			}

			// If ImageAutoMod is true/enabled, moderate images
			for _, imgData := range images {
				isSafe, err := AnalyzeImageWithGoogleVision(imgData)
				if err != nil {
					fmt.Printf("[WARN] Google Vision API error: %v — rejecting image safely\n", err)
					return "0", nil
				}
				if !isSafe {
					fmt.Println("[INFO] Image rejected by Google Vision API")
					return "0", nil
				}
			}
		}
	}

	clean, err := AnalyzeWithGoogleNLP(cleanedContent)
	if err != nil {
		// Google NLP unavailable (e.g. missing API key); fall back to allow
		fmt.Printf("[WARN] Google NLP unavailable: %v — defaulting to allow\n", err)
		return "1", nil
	}
	if clean {
		return "1", nil
	}
	return "0", nil
}

func AnalyzeImageWithGoogleVision(base64Uri string) (bool, error) {
	apiKey, err := GetGoogleApiKey()
	if err != nil {
		return false, err
	}

	if apiKey == "" {
		logger.LogToFile("[DEBUG]Missing GOOGLE NLP API KEY for Vision")
		return false, fmt.Errorf("missing GOOGLE_NLP_API_KEY in environment")
	}

	// Strip the "data:image/...;base64," prefix
	parts := strings.SplitN(base64Uri, ",", 2)
	base64Payload := base64Uri
	if len(parts) == 2 {
		base64Payload = parts[1]
	}

	payload := map[string]interface{}{
		"requests": []map[string]interface{}{
			{
				"image": map[string]interface{}{
					"content": base64Payload,
				},
				"features": []map[string]interface{}{
					{
						"type": "SAFE_SEARCH_DETECTION",
					},
				},
			},
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}

	url := fmt.Sprintf("https://vision.googleapis.com/v1/images:annotate?key=%s", apiKey)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("Google Vision API error: %s", string(body))
	}

	var result struct {
		Responses []struct {
			SafeSearchAnnotation struct {
				Adult    string `json:"adult"`
				Spoof    string `json:"spoof"`
				Medical  string `json:"medical"`
				Violence string `json:"violence"`
				Racy     string `json:"racy"`
			} `json:"safeSearchAnnotation"`
		} `json:"responses"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return false, err
	}

	if len(result.Responses) == 0 {
		return true, nil // No annotation found, default allow
	}

	annotation := result.Responses[0].SafeSearchAnnotation

	// Check for LIKELY or VERY_LIKELY flags
	unsafeFlags := []string{"LIKELY", "VERY_LIKELY"}
	for _, flag := range unsafeFlags {
		if annotation.Adult == flag || annotation.Violence == flag || annotation.Racy == flag {
			fmt.Printf("Blocked image. Adult: %s, Violence: %s, Racy: %s\n", annotation.Adult, annotation.Violence, annotation.Racy)
			return false, nil
		}
	}

	return true, nil
}

func ManModerateMsg(cert types.MsgCert) (*models.ModResponse, error) {
	resp, err := moddb.StoreMsgResult(cert)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func SelectModel(method string) (ModelFunc, error) {
	return AnalyzeWithGoogleNLP, nil
}

func removeLinks(input string) string {
	urlRegex := `https?://[^\s]+|ftp://[^\s]+|www\.[^\s]+`
	re := regexp.MustCompile(urlRegex)
	return re.ReplaceAllString(input, "")
}

func AnalyzeContent(content string, fn ModelFunc) (string, error) {
	cleanedContent := removeLinks(content)
	clean, err := fn(cleanedContent)
	if err != nil {
		return "error", err
	}
	if clean {
		return "1", nil
	}
	return "0", nil
}

func ParseThresholds() map[string]float64 {
	config, err := ReadModConfigFile()
	if err != nil {
		fmt.Println("Error loading thresholds:", err)
		return map[string]float64{}
	}

	thresholds := make(map[string]float64)

	if config.Thresholds == "" {
		logger.LogToFile("[DEBUG]No thresholds found")
		fmt.Println("No thresholds found in JSON")
		return thresholds
	}

	fmt.Println("Loaded THRESHOLDS from JSON:", config.Thresholds)

	pairs := strings.Split(config.Thresholds, ",")
	for _, pair := range pairs {
		kv := strings.Split(pair, ":")
		if len(kv) == 2 {
			conf, err := strconv.ParseFloat(kv[1], 64)
			if err == nil {
				thresholds[kv[0]] = conf
			} else {
				fmt.Printf("Invalid threshold value for %s: %s\n", kv[0], kv[1])
			}
		}
	}

	return thresholds
}

func AnalyzeWithGoogleNLP(content string) (bool, error) {
	apiKey, err := GetGoogleApiKey()
	if err != nil {
		return false, err
	}

	if apiKey == "" {
		logger.LogToFile("[DEBUG]Missing GOOGLE NLP API KEY")
		return false, fmt.Errorf("missing GOOGLE_NLP_API_KEY in environment")
	}

	payload := map[string]interface{}{
		"document": map[string]interface{}{
			"type":    "PLAIN_TEXT",
			"content": content,
		},
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}

	url := fmt.Sprintf("https://language.googleapis.com/v1/documents:moderateText?key=%s", apiKey)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("Google NLP API error: %s", string(body))
	}

	var result struct {
		ModerationCategories []struct {
			Name       string  `json:"name"`
			Confidence float64 `json:"confidence"`
		} `json:"moderationCategories"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return false, err
	}

	thresholds := ParseThresholds()

	fmt.Println(result)

	for _, cat := range result.ModerationCategories {
		if th, exists := thresholds[cat.Name]; exists {
			if cat.Confidence >= th {
				fmt.Printf("Blocked category: %s (%.2f ≥ %.2f)\n", cat.Name, cat.Confidence, th)
				return false, nil
			}
		}
	}
	return true, nil
}

func AppendToModLog(msg models.UserMsg, status string) error {
	entry := struct {
		PublicKey string `json:"public_key"`
		Content   string `json:"content"`
		Timestamp int64  `json:"timestamp"`
		Status    string `json:"status"`
	}{
		Content:   msg.Content,
		Timestamp: msg.TimeStamp,
		Status:    status,
	}

	path := filepath.Join(cache.GetCacheDir(), "modlog.json")

	var entries []any

	data, err := os.ReadFile(path)
	if err == nil && len(data) > 0 {
		err = json.Unmarshal(data, &entries)
		if err != nil {
			return fmt.Errorf("error parsing existing modlog.json: %w", err)
		}
	}

	entries = append(entries, entry)

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	return encoder.Encode(entries)
}
