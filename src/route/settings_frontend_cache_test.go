package route

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsFrontendCachePolicy(t *testing.T) {
	assets, err := filepath.Glob(filepath.Join("..", "www", "assets", "lib-*.js"))
	if err != nil {
		t.Fatalf("glob shared frontend assets: %v", err)
	}

	var assetPath string
	var assetBody []byte
	for _, candidate := range assets {
		body, readErr := os.ReadFile(candidate)
		if readErr != nil {
			t.Fatalf("read frontend asset %s: %v", candidate, readErr)
		}
		if strings.Contains(string(body), "/admin/api/v1/settings") {
			if assetPath != "" {
				t.Fatalf("multiple shared settings assets found: %s and %s", assetPath, candidate)
			}
			assetPath = candidate
			assetBody = body
		}
	}
	if assetPath == "" {
		t.Fatal("shared settings frontend asset not found")
	}
	assetName := filepath.Base(assetPath)
	for _, pagePrefix := range []string{"settings-", "telegram-", "system-", "pay-", "epay-", "okpay-"} {
		pageAssets, globErr := filepath.Glob(filepath.Join("..", "www", "assets", pagePrefix+"*.js"))
		if globErr != nil {
			t.Fatalf("glob %s page asset: %v", pagePrefix, globErr)
		}
		if len(pageAssets) != 1 {
			t.Fatalf("expected one %s page asset, got %v", pagePrefix, pageAssets)
		}
		pageBody, readErr := os.ReadFile(pageAssets[0])
		if readErr != nil {
			t.Fatalf("read page asset %s: %v", pageAssets[0], readErr)
		}
		if !strings.Contains(string(pageBody), `./`+assetName) {
			t.Fatalf("settings page asset %s does not use shared query asset %s", pageAssets[0], assetName)
		}
	}

	cachePolicy := `{staleTime:0,refetchOnMount:` + "`always`" + `,gcTime:0}`
	if !strings.Contains(string(assetBody), cachePolicy) {
		t.Fatalf("shared settings query in %s is missing cache policy %s", assetPath, cachePolicy)
	}

	swBody, err := os.ReadFile(filepath.Join("..", "www", "sw.js"))
	if err != nil {
		t.Fatalf("read service worker: %v", err)
	}
	revision := fmt.Sprintf("%x", sha256.Sum256(assetBody))
	manifestEntry := fmt.Sprintf(`{url:"assets/%s",revision:"%s"}`, assetName, revision)
	if !strings.Contains(string(swBody), manifestEntry) {
		t.Fatalf("service worker is missing revisioned settings asset entry %s", manifestEntry)
	}
}
