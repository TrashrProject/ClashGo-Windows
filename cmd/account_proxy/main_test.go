package main

import (
	"net/http"
	"testing"
	"time"
)

func TestNormalizeTag(t *testing.T) {
	got, err := normalizeTag(" 899qupv2q ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "#899QUPV2Q" {
		t.Fatalf("tag=%q want #899QUPV2Q", got)
	}

	if _, err := normalizeTag("#BAD-TAG"); err == nil {
		t.Fatal("expected invalid character error")
	}
}

func TestProfileCacheExpires(t *testing.T) {
	c := &profileCache{entries: make(map[string]cacheEntry)}
	c.put("#ABC123", http.StatusOK, []byte(`{"tag":"#ABC123"}`), 20*time.Millisecond)

	if _, ok := c.get("#ABC123"); !ok {
		t.Fatal("expected cache hit")
	}

	time.Sleep(30 * time.Millisecond)
	if _, ok := c.get("#ABC123"); ok {
		t.Fatal("expected expired cache miss")
	}
}
