// Package telemetry 封装 OpenTelemetry SDK 初始化逻辑，
// 为 AgentFlow 提供集中式的 TracerProvider 和 MeterProvider 配置。
// 当遥测功能禁用时，使用 noop 实现，不连接任何外部服务。
package telemetry
