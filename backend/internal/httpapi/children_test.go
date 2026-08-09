package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/family-habit/family-habit/backend/internal/children"
)

func TestValidateChild(t *testing.T) {
	validPIN := "1234"
	if issues := validateChild(" Sam ", "fox", "#12Ab34", &validPIN); len(issues) != 0 {
		t.Fatalf("valid child issues = %+v", issues)
	}
	badPIN := "12ab"
	if issues := validateChild(" ", "remote-url", "red", &badPIN); len(issues) != 4 {
		t.Fatalf("invalid child issues = %+v", issues)
	}
}

func TestChildPickerProjectionContainsNoAdministrativeOrSecretFields(t *testing.T) {
	encoded, err := json.Marshal(childPickerJSON(children.Child{
		ID: "child-id", Nickname: "Sam", Avatar: "fox", Color: "#112233",
		PINEnabled: true, Active: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, forbidden := range []string{"pinEnabled", "active", "createdAt", "updatedAt", "hash"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("picker projection leaked %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, `"pinRequired":true`) {
		t.Fatalf("picker projection missing PIN challenge indicator: %s", text)
	}
}
