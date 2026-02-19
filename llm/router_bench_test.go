package llm

import (
	"context"
	"testing"
)

// =============================================================================
// 🧪 LLM Router 性能基准测试
// =============================================================================

// BenchmarkMultiProviderRouter_SelectProvider 测试路由选择性能
func BenchmarkMultiProviderRouter_SelectProvider(b *testing.B) {
	// 创建路由器（使用内存数据库）
	router := setupBenchmarkRouter(b)

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := router.SelectProviderWithModel(ctx, "gpt-4o", StrategyCostBased)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMultiProviderRouter_SelectProvider_Parallel 并发路由选择
func BenchmarkMultiProviderRouter_SelectProvider_Parallel(b *testing.B) {
	router := setupBenchmarkRouter(b)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := router.SelectProviderWithModel(ctx, "gpt-4o", StrategyCostBased)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkMultiProviderRouter_RouteLoop 测试连续路由性能
func BenchmarkMultiProviderRouter_RouteLoop(b *testing.B) {
	router := setupBenchmarkRouter(b)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := router.SelectProviderWithModel(ctx, "gpt-4o", StrategyCostBased)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMultiProviderRouter_RouteLoop_Parallel 并发路由请求
func BenchmarkMultiProviderRouter_RouteLoop_Parallel(b *testing.B) {
	router := setupBenchmarkRouter(b)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := router.SelectProviderWithModel(ctx, "gpt-4o", StrategyCostBased)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkMultiProviderRouter_StrategySwitch 测试策略切换性能
func BenchmarkMultiProviderRouter_StrategySwitch(b *testing.B) {
	router := setupBenchmarkRouter(b)
	ctx := context.Background()
	strategies := []RoutingStrategy{StrategyCostBased, StrategyHealthBased, StrategyQPSBased}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		strategy := strategies[i%len(strategies)]
		_, err := router.SelectProviderWithModel(ctx, "gpt-4o", strategy)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// =============================================================================
// 🔧 辅助函数
// =============================================================================

// setupBenchmarkRouter 创建基准测试用的路由器
func setupBenchmarkRouter(b *testing.B) *MultiProviderRouter {
	b.Helper()

	// 使用内存数据库（需要实现）
	// TODO: 实现 setupInMemoryDB 或使用 mock
	// db := setupInMemoryDB(b)

	// 暂时跳过，因为需要完整的数据库设置
	b.Skip("需要完整的数据库设置")

	return nil
}

// =============================================================================
// 📊 基准测试结果示例
// =============================================================================

/*
运行基准测试：
go test -bench=BenchmarkMultiProviderRouter -benchmem -benchtime=10s

预期结果（参考）：
BenchmarkMultiProviderRouter_SelectProvider-8                  	 1000000	      1200 ns/op	     512 B/op	      10 allocs/op
BenchmarkMultiProviderRouter_SelectProvider_Parallel-8         	 5000000	       300 ns/op	     256 B/op	       5 allocs/op
BenchmarkMultiProviderRouter_Completion-8                      	  500000	      2500 ns/op	    1024 B/op	      20 allocs/op
BenchmarkMultiProviderRouter_Completion_Parallel-8             	 2000000	       800 ns/op	     512 B/op	      10 allocs/op
BenchmarkMultiProviderRouter_HealthCheck-8                     	 2000000	       600 ns/op	     256 B/op	       8 allocs/op

性能目标：
- 路由选择：< 2ms
- 完整请求：< 5ms（不含实际 LLM 调用）
- 健康检查：< 1ms
- 并发性能：线性扩展
*/
