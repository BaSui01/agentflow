// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 threed 提供统一的 3D 模型生成接口，支持文本转 3D 和图像转 3D 两种模式，
并适配 Meshy 与 Tripo3D 两个主流服务商。

# 概述

本包为上层业务屏蔽不同 3D 生成服务商在 API 协议、任务轮询和输出格式上的差异，
对外暴露一致的请求/响应模型。典型使用场景包括：

  - 通过文本描述生成 3D 模型（text-to-3D）。
  - 通过单张或多视角图像生成 3D 模型（image-to-3D / multiview-to-3D）。
  - 获取 GLB、FBX、OBJ、USDZ 等多种格式的模型文件。

# 核心接口

  - ThreeDProvider — 3D 生成的统一抽象，包含 Name() 与 Generate() 方法。
  - GenerateRequest — 生成请求，支持 prompt、image、format、quality 等参数。
  - GenerateResponse — 生成响应，包含模型下载链接、缩略图与用量统计。
  - ModelData — 单个生成模型的元数据（URL、格式、缩略图）。

# 主要能力

  - Meshy 适配：通过 MeshyProvider 对接 Meshy API，支持 text-to-3D 与
    image-to-3D，可选 preview / refine 质量等级。
  - Tripo3D 适配：通过 TripoProvider 对接 Tripo3D API，支持 text-to-model、
    image-to-model 与 multiview-to-model 三种任务类型。
  - 异步轮询：两个 Provider 均内置任务状态轮询，调用方无需关心异步细节。
  - 多格式输出：支持 GLB、FBX、OBJ、USDZ 格式选择。
*/
package threed
