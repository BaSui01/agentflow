# 备份和恢复

> AgentFlow 数据备份与灾难恢复指南

## 概述

本文档介绍 AgentFlow 生产环境中需要备份的数据组件及恢复流程。

## 需要备份的数据

### 1. 数据库

AgentFlow 使用关系型数据库（PostgreSQL/MySQL/SQLite）存储元数据：

- Provider 配置与 API Key
- 模型映射关系
- 路由规则与策略

```bash
# PostgreSQL 备份示例
pg_dump -h localhost -U agentflow agentflow_db > backup_$(date +%Y%m%d).sql

# MySQL 备份示例
mysqldump -u agentflow -p agentflow_db > backup_$(date +%Y%m%d).sql
```

### 2. 向量数据库

RAG 系统使用的向量数据需要单独备份，具体方式取决于所用的向量数据库：

- **Qdrant**: 使用 Snapshot API
- **Pinecone**: 使用 Collections 备份
- **Milvus**: 使用 backup 工具

### 3. 配置文件

```bash
# 备份配置目录
tar -czf config_backup_$(date +%Y%m%d).tar.gz config/
```

## 恢复流程

1. 停止 AgentFlow 服务
2. 恢复数据库备份
3. 恢复向量数据库快照
4. 恢复配置文件
5. 验证配置完整性
6. 重启服务并验证健康状态

## 定期备份建议

| 数据类型 | 建议频率 | 保留周期 |
|---------|---------|---------|
| 数据库 | 每日 | 30 天 |
| 向量数据库 | 每周 | 90 天 |
| 配置文件 | 每次变更 | 永久 |

## 相关文档

- [Kubernetes 部署](./kubernetes.md)
- [监控和告警配置](./monitoring.md)
