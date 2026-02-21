// 版权所有 2024 AgentFlow Authors. 保留所有权利。
// 此源代码的使用由 MIT 许可规范，该许可可以
// 在 LICENSE 文件中找到。

/*
示例 21_research_workflow 演示了基于 AgentFlow DAG 工作流引擎的科研自动化流水线。

# 演示内容

本示例模拟完整的科研工作流，包含七个阶段：

  - Literature Collection：文献收集，从 arXiv、IEEE 等来源检索论文
  - Quality Filtering：质量过滤，按评分阈值筛选高质量文献
  - Idea Generation：创意生成，基于文献分析提出新研究方向，
    评估 Novelty、Feasibility 和 Impact 三维指标
  - Design：方案设计，生成架构、组件、技术栈与评估指标
  - Implementation：代码实现，生成源码与测试文件
  - Validation：实验验证，对比 Baseline 评估改进幅度
  - Report Generation：报告生成，输出包含摘要、方法、结果的结构化研究报告

各阶段按 DAG 拓扑顺序串行执行，生产环境中可通过 DAGBuilder 配置并行分支。

# 运行方式

	go run .
*/
package main
