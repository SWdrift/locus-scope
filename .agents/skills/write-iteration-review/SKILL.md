---
name: write-iteration-review
description: "按用户指定的 Git 提交范围创建或重写任务级迭代复盘，清晰呈现范围终点的架构、提交的业务含义、要求达成情况、验证证据与未完成边界。用于用户要求生成迭代复盘、阶段总结、交付复盘，或要求根据一批 commit 更新 document/iteration-reviews 下的 Markdown 文档时。"
---

# Write Iteration Review

根据用户指定的提交范围编写任务级迭代复盘。先理解这批提交作为一个整体改变了什么，再组织文档，不以 commit message 或文件清单代替结论。

## 取材

- 确认实际提交范围和事实终点。用户表达“从 A 到 B”时包含两端；用户给出 Git revision range 时沿用其语义。
- 阅读提交、关键 diff，以及相关的架构、要求和验证材料。区分已经实现的事实、目标、未验证结论和范围外变化。
- 未指定输出文件时，沿用仓库 `document/iteration-reviews/` 的编号与命名方式。

## 成文

- 先阅读 [iteration-review-shape.md](references/iteration-review-shape.md)；在本仓库同时以 `document/iteration-reviews/0000-Earth-clear统一输运迭代复盘.md` 作为成文参照。
- `Scope` 说明提交范围。`Architecture`、`Impact`、`Delivery`、`Evidence`、`Next Steps` 按实际内容使用，章节内部形式和详略以清晰为准。
- 先给总体把握，在重要部分给出足以理解原因、选择和边界的细节。不要把文档写成平均展开的摘要或纯客观事实清单。

## 事实边界

- 以提交范围终点为准，不混入后续实现或未提交改动。
- 不把目标、命名、截图或有限样本提升为它们不能支持的完成结论。
- 迭代复盘记录本轮结论，不替代当前架构真相和活跃任务文档。
