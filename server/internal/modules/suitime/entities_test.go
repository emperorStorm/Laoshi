package suitime

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func candidate(entity, id, data string) change {
	return change{Entity: entity, ID: id, UpdatedAt: 100, DeviceID: "original-device", Data: json.RawMessage(data)}
}

func TestClientEntityContracts(t *testing.T) {
	cases := []struct{ entity, id, data string }{
		{"category", "work", `{"name":"工作分类","color":"#ffffff","createdAt":10,"sortOrder":0}`},
		{"task", "task", `{"title":"事项","status":"todo","notes":"","createdAt":10,"plannedTime":null,"parentTaskId":null}`},
		{"task", "repeating", `{"title":"重复事项","status":"done","repeatRule":"{\"kind\":\"weekly_slots\",\"weekdays\":[2]}","occurrenceOverrides":"{\"2026-01-01\":{\"status\":\"done\",\"notes\":\"\"}}"}`},
		{"anniversary", "ann", `{"title":"纪念日","kind":"anniversary","date":"2026-01-01","notes":"","photos":[],"coverIndex":0,"createdAt":10}`},
		{"anniversary_record", "ann@2026-01-01", `{"anniversaryId":"ann","date":"2026-01-01","title":"纪念日","kind":"anniversary","originalDate":"2025-01-01","notes":"","confirmedAt":null,"createdAt":10}`},
		{"setting", "show_completed.tasks", `{"value":true,"createdAt":10}`},
		{"setting", "mobile.profile", `{"value":{"name":"用户"}}`},
	}
	for _, tt := range cases {
		t.Run(tt.id, func(t *testing.T) {
			got, err := normalizeChange(candidate(tt.entity, tt.id, tt.data), "batch-device", 1000, 6<<20)
			if err != nil {
				t.Fatal(err)
			}
			if got.DeviceID != "original-device" {
				t.Fatal("lost originating device")
			}
			if strings.Contains(tt.data, `"createdAt":10`) && got.Data["createdAt"] != json.Number("10") {
				t.Fatalf("lost createdAt: %v", got.Data)
			}
		})
	}
}

func TestInvalidEntityFields(t *testing.T) {
	cases := []struct{ entity, id, data string }{
		{"task", "t", `{"title":"x","status":"unknown"}`},
		{"task", "t", `{"title":"x","status":"todo","priority":3}`},
		{"task", "t", `{"title":"x","status":"todo","categoryId":true}`},
		{"task", "t", `{"title":"x","status":"todo","parentTaskId":"t"}`},
		{"task", "t", `{"title":"x","status":"todo","completedAt":"100"}`},
		{"task", "t", `{"title":"x","status":"todo","sortOrder":1.2}`},
		{"task", "t", `{"title":"x","status":"todo","repeatRule":"[]"}`},
		{"task", "t", `{"title":"x","status":"todo","occurrenceOverrides":"[]"}`},
		{"task", "t", `{"title":"x","status":"todo","scheduleKind":"range"}`},
		{"anniversary", "a", `{"title":"x","kind":"fake","date":"2026-01-01"}`},
		{"anniversary", "a", `{"title":"x","kind":"birthday","date":"2026-02-30"}`},
		{"anniversary", "a", `{"title":"x","kind":"birthday","date":"2026-01-01","pinned":1}`},
		{"anniversary", "a", `{"title":"x","kind":"birthday","date":"2026-01-01","coverIndex":2}`},
		{"anniversary", "a", `{"title":"x","kind":"birthday","date":"2026-01-01","photos":["https://example.com/a.jpg"]}`},
		{"anniversary_record", "wrong", `{"anniversaryId":"a","date":"2026-01-01","title":"x","kind":"birthday","originalDate":"2025-01-01"}`},
		{"setting", "show_completed.tasks", `{"value":"yes"}`},
		{"setting", "secret", `{"value":true}`},
	}
	for i, tt := range cases {
		if _, err := normalizeChange(candidate(tt.entity, tt.id, tt.data), "device", 1000, 6<<20); err == nil {
			t.Errorf("case %d accepted invalid fields", i)
		}
	}
}

func TestLegacySettingAndTombstone(t *testing.T) {
	item := candidate("setting", "show_completed.tasks", `{"value":false,"createdAt":0}`)
	item.UpdatedAt = 0
	got, err := normalizeChange(item, "device", 1000, 6<<20)
	if err != nil || got.UpdatedAt != 1 || got.Data["createdAt"] != json.Number("1") {
		t.Fatalf("legacy preference: %+v %v", got, err)
	}
	item = candidate("setting", "mobile.new_task_defaults", `{"value":null}`)
	deleted := int64(100)
	item.DeletedAt = &deleted
	if _, err := normalizeChange(item, "device", 1000, 6<<20); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeRejectsMalformedBodies(t *testing.T) {
	for _, body := range []string{"", `{`, `{"username":"a"} {}`, `{"unknown":1}`} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		var input credentials
		if decode(w, r, &input) || w.Code != 400 {
			t.Fatalf("accepted %q: %d", body, w.Code)
		}
	}
}

func TestPasswordsAndTokens(t *testing.T) {
	hash, err := hashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPassword(hash, "correct-password") || verifyPassword(hash, "wrong-password") || verifyPassword("$argon2id$bad", "x") {
		t.Fatal("password verification failed")
	}
	a, _ := newToken()
	b, _ := newToken()
	if a == b || len(a) < 40 || tokenDigest(a) == a {
		t.Fatal("invalid tokens")
	}
	if readBearer("bEaReR token") != "token" || readBearer("Bearer token extra") != "" {
		t.Fatal("invalid bearer parsing")
	}
}
