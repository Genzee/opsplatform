package fixture

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Genzee/opsplatform/internal/repository"
)

func TestRejectMalformedAndUnknownFixtureFields(t *testing.T) {
	for _, input := range []string{`null`, `{} {}`, `{"resoruces":[]}`, `{"resources":[{"ref":{"provider":"p","kind":"x","native_id":"id"},"name":"x","secret_typo":true}]}`} {
		path := filepath.Join(t.TempDir(), "fixture.json")
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		m := repository.NewMemory()
		if err := Load(context.Background(), path, m); err == nil {
			t.Fatalf("accepted invalid fixture %s", input)
		}
		s, _ := m.Snapshot(context.Background())
		if len(s.Resources) != 0 {
			t.Fatal("invalid fixture partially committed")
		}
	}
}
