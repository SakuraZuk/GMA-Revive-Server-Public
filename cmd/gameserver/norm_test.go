package main

import (
	"encoding/json"
	"testing"
)

func TestNormalizeArgsNumericMap(t *testing.T) {
	raw := []byte(`{"_0":{"account":"2210551091","hostnum":10001}}`)
	args, err := normalizeArgs(raw)
	if err != nil || len(args) != 1 {
		t.Fatalf("args=%v err=%v", args, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(args[0], &doc); err != nil {
		t.Fatal(err)
	}
	if doc["account"] != "2210551091" {
		t.Fatalf("account=%v", doc["account"])
	}
}
