package llm

// 已删除 : 此文件包含遗留的路由器执行 。
// 使用多提供者路透(router multi productor.go)代替多提供者支持.
//
// 遗留的路由器 设计为单一的 提供者/模式建筑,
// 它与一个新的多提供者数据模型不相容,在一个模型中
// 可由多个提供者提供。
//
// 移徙指南:
// - 将新路透号( ) 替换为新多路透号( )
// - 使用SelectProviderWith Model () 而不是SelectProvider ()
// - API 关键集合现在由每个提供者管理
