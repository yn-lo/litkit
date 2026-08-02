package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"litkit/internal/config"
	"litkit/internal/lint"
	"litkit/internal/storage"
)

// envTemplate 工作目录 .env 模板（litkit init 生成，FR-CONFIG-03）。
const envTemplate = "# litkit 工作目录配置（由 litkit init 生成，FR-CONFIG-03）\n" +
	"# 所有默认值均可在此调整；AI 经本目录的 AGENTS.md 获知这些配置\n" +
	"\n" +
	"# 工作目录（必填：未设置时 litkit 拒绝执行，FR-LIB-03）\n" +
	"LITKIT_WORK_DIR=.\n" +
	"# 每源默认检索条数\n" +
	"LITKIT_DEFAULT_MAX_RESULTS=5\n" +
	"# 默认检索时间范围（最近 N 年，FR-SEARCH-13）\n" +
	"LITKIT_DEFAULT_RECENT_YEARS=3\n" +
	"# 默认检索等级：tiab（题目+摘要+关键词）| full（全文，FR-SEARCH-12）\n" +
	"LITKIT_DEFAULT_SEARCH_MODE=tiab\n" +
	"# 可选：Semantic Scholar API key（配置后提速，避免共享池 429）\n" +
	"# LITKIT_SEMANTIC_SCHOLAR_API_KEY=\n"

// agentsHead AGENTS.md 固定头部（配置/命令/检索策略）。
const agentsHead = "# litkit — AI agent 使用说明\n" +
	"\n" +
	"> 本文件由 `litkit init` 生成，AI 在本工作目录工作时自动加载。\n" +
	"\n" +
	"## 配置\n" +
	"\n" +
	"- 运行 litkit 前须设置 `LITKIT_WORK_DIR=<本目录>`（未设置时 init/search/lib 拒绝执行，FR-LIB-03）\n" +
	"- 配置文件：`.env`（本目录）；所有默认值可在其中调整\n" +
	"- 文献库：`litkit.db`（本目录）；删除工作目录即删除文献库（无 TTL）\n" +
	"- 撰写规范：`.litkit/`（rules.md 规则 / checklist.md 人工清单 / specs/manuscript-spec.yaml 阈值配置）\n" +
	"\n" +
	"## 核心命令\n" +
	"\n" +
	"- `litkit search \"<英文关键词>\"` 跨源检索（自动入库，输出精简视图）\n" +
	"  - `-s arxiv,pubmed` 源过滤；`-n 5` 每源条数\n" +
	"  - `--mode tiab|full` 检索等级（默认 tiab）\n" +
	"  - `--years N` / `--since YEAR` 时间范围\n" +
	"  - `--full` 完整元数据与错误\n" +
	"- `litkit sources` 查看可用源\n" +
	"- `litkit lib list / search / rm <citeKey> / stats / path` 文献库管理\n" +
	"- `litkit verify manuscript/*.md` 成稿后机械化验证（三要素：问题/修复/规则编号）\n" +
	"\n" +
	"## 检索策略\n" +
	"\n" +
	"- 检索词**必须英文**（各源英文语料为主，中文命中率极低，FR-SEARCH-11）\n" +
	"- 默认 tiab=题目+摘要+关键词；结果不足时用 `--mode full` 全文检索\n" +
	"- 默认最近 3 年；可 `--years 10` 或 `--since 2015` 放宽\n" +
	"- 单源失败不阻断整体：`errors` 字段说明原因，退出码 3 = 部分源成功\n"

// agentsTail AGENTS.md 固定尾部（验证与引用）。
const agentsTail = "## 验证与引用\n" +
	"\n" +
	"- 成稿后运行 `litkit verify manuscript/*.md`；人工复核 `.litkit/checklist.md`\n" +
	"- 引用时写 `[cite:<citeKey>]` 占位符，不展开元数据；manuscript 流水线按 citeKey\n" +
	"  生成 GB/T 7714 / APA / IEEE 规范引用\n"

// 文件权限常量（mnd：避免魔法值）。
const (
	initDirPerm  = 0o750
	initFilePerm = 0o600
)

// newInitCmd 构造 `litkit init` 子命令。
//
// 生成 .env + .litkit/（rules/checklist/specs/verifier）+ AGENTS.md（含撰写硬性规定），
// 并初始化 litkit.db。已存在文件默认不覆盖（--force 覆盖）。
// --type/--lang 仅在首次生成 yaml 时生效；--refresh 按现有 yaml 重新渲染 AGENTS.md。
func newInitCmd(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init [--type review|empirical] [--lang zh|en] [--refresh]",
		Short: "初始化工作目录（.env + .litkit/ + AGENTS.md + litkit.db）",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			force, _ := cmd.Flags().GetBool("force")
			refresh, _ := cmd.Flags().GetBool("refresh")
			paperType, _ := cmd.Flags().GetString("type")
			lang, _ := cmd.Flags().GetString("lang")
			// 工作目录必须显式设置（FR-LIB-03）：拒绝就地生成污染 CWD
			if err := requireWorkDir(cfg); err != nil {
				return err
			}
			return initWorkdir(cfg.WorkDir, force, refresh, paperType, lang)
		},
	}
	cmd.Flags().Bool("force", false, "覆盖已存在的 .env / .litkit / AGENTS.md")
	cmd.Flags().Bool("refresh", false, "按现有 manuscript-spec.yaml 重新生成 AGENTS.md 撰写段")
	cmd.Flags().String("type", lint.PaperTypeEmpirical, "论文类型：review（综述）| empirical（四段式实证）")
	cmd.Flags().String("lang", lint.LangZH, "撰写语言：zh | en")
	return cmd
}

// initWorkdir 生成 .env / .litkit / AGENTS.md 并初始化 litkit.db。
//
// 文件权限 0600：.env 与 verifier_models.json 可能含 API key，须防其他用户读取。
func initWorkdir(dir string, force, refresh bool, paperType, lang string) error {
	if err := os.MkdirAll(dir, initDirPerm); err != nil {
		return fmt.Errorf("init: mkdir %s: %w", dir, err)
	}

	created := []string{}
	if ok, err := writeIfAbsent(filepath.Join(dir, ".env"), envTemplate, force); err != nil {
		return err
	} else if ok {
		created = append(created, ".env")
	}

	// 生成 .litkit/（模板，go:embed）
	harnessCreated, err := lint.InitHarness(dir, force)
	if err != nil {
		return err
	}
	created = append(created, harnessCreated...)

	// 解析撰写规范：--type/--lang 仅在 yaml 缺失或本次新建（或 --force）时生效；
	// 其余情况（yaml 已存在且非本次创建）走现有 yaml，flag 不生效
	specPath := lint.SpecPath(dir)
	yamlJustCreated := false
	specRel := filepath.ToSlash(filepath.Join(lint.LitkitDir, "specs", "manuscript-spec.yaml"))
	for _, c := range harnessCreated {
		if filepath.ToSlash(c) == specRel {
			yamlJustCreated = true
		}
	}
	var spec *lint.ManuscriptSpec
	switch {
	case refresh:
		spec, err = lint.LoadSpec(specPath)
	case fileExists(specPath) && !force && !yamlJustCreated:
		spec, err = lint.LoadSpec(specPath)
	default:
		spec = lint.DefaultSpec()
		spec.PaperType = paperType
		spec.Lang = lang
		// 仅 flag 非默认时才覆盖模板 yaml（保留带注释的模板；marshal 输出无注释）
		if spec.PaperType != lint.PaperTypeEmpirical || spec.Lang != lint.LangZH {
			if err = lint.WriteSpec(specPath, spec); err != nil {
				return err
			}
		}
	}
	if err != nil {
		return err
	}

	// 生成 AGENTS.md = 固定头 + 撰写硬性规定（事前指导）+ 固定尾
	// refresh 模式强制重写（--refresh 的语义就是重新渲染撰写段）
	agents := agentsHead + "\n" + lint.RenderWritingRules(spec) + "\n" + agentsTail
	if ok, err := writeIfAbsent(filepath.Join(dir, "AGENTS.md"), agents, force || refresh); err != nil {
		return err
	} else if ok {
		created = append(created, "AGENTS.md")
	}

	// 初始化文献库（幂等：已存在则跳过建表）
	dbPath := filepath.Join(dir, storage.DefaultDBName)
	dbExisted := fileExists(dbPath)
	store, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("init: storage open: %w", err)
	}
	defer func() { _ = store.Close() }()
	if !dbExisted {
		created = append(created, storage.DefaultDBName)
	}

	return printJSON(map[string]any{
		"workDir": dir,
		"created": created,
	})
}

// writeIfAbsent 写文件；已存在且未 force 时跳过。返回是否创建。
func writeIfAbsent(path, content string, force bool) (bool, error) {
	if fileExists(path) && !force {
		return false, nil
	}
	if err := os.WriteFile(path, []byte(content), initFilePerm); err != nil {
		return false, fmt.Errorf("init: write %s: %w", path, err)
	}
	return true, nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
