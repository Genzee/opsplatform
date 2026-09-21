package fixture

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/Genzee/opsplatform/internal/repository"
	"github.com/Genzee/opsplatform/pkg/model"
)

func Load(ctx context.Context, path string, store *repository.Memory) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (4<<20)+1))
	if err != nil {
		return err
	}
	if len(data) > 4<<20 {
		return fmt.Errorf("fixture exceeds 4 MiB")
	}
	var batch model.Batch
	// Unmarshal rejects trailing documents; unknown fields are rejected below.
	if err := strictDecode(data, &batch); err != nil {
		return err
	}
	return store.Apply(ctx, batch)
}
