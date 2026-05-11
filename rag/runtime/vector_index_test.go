package runtime

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestHNSWIndex_ConcurrentAddAndSearch(t *testing.T) {
	cfg := DefaultHNSWConfig()
	cfg.M = 4
	cfg.EfConstruction = 16
	cfg.EfSearch = 16

	idx := NewHNSWIndex(cfg, zap.NewNop())
	vectors := make([][]float64, 0, 16)
	ids := make([]string, 0, 16)
	for i := 0; i < 16; i++ {
		vectors = append(vectors, hnswTestVector(i))
		ids = append(ids, fmt.Sprintf("seed-%d", i))
	}
	require.NoError(t, idx.Build(vectors, ids))

	start := make(chan struct{})
	errCh := make(chan error, 128)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 64; i++ {
			id := fmt.Sprintf("added-%d", i)
			if err := idx.Add(hnswTestVector(100+i), id); err != nil {
				errCh <- err
				return
			}
		}
	}()

	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			for i := 0; i < 64; i++ {
				if _, err := idx.Search(hnswTestVector(worker*1000+i), 5); err != nil {
					errCh <- err
					return
				}
			}
		}(worker)
	}

	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
	require.Equal(t, 80, idx.Size())
}

func hnswTestVector(seed int) []float64 {
	x := float64(seed + 1)
	return []float64{x, x * 0.5, x * 0.25, 1}
}
