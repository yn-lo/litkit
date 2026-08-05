package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// agentsHead 与 agentsTail 已移至 internal/lint/agents.go（CLI 与 MCP 共用，FR-IFACE-03）。

// 文件权限常量（mnd：避免魔法值）。
const (
	initDirPerm  = 0o750
	initFilePerm = 0o600
)

// newInitCmd 构造 `litkit init` 子命令。
//
// 生成 .env + .litkit/（rules/checklist/specs/verifier）+ AGENTS.md（含撰写硬性规定），
// 并初始化 litkit.db。已存在文件默认不覆盖（--force 覆盖）。
// --type/--lang/--journal 仅在首次生成 yaml 时生效；--refresh 按现有 yaml 重新渲染 AGENTS.md。
func newInitCmd(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init [--type review|empirical] [--lang zh|en] [--journal NAME] [--refresh]",
		Short: "初始化工作目录（.env + .litkit/ + AGENTS.md + litkit.db）",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			force, _ := cmd.Flags().GetBool("force")
			refresh, _ := cmd.Flags().GetBool("refresh")
			paperType, _ := cmd.Flags().GetString("type")
			lang, _ := cmd.Flags().GetString("lang")
			journal, _ := cmd.Flags().GetString("journal")

			// 交互式向导：旗标未显式传值且 stdin 是终端时，进入问答
			if !cmd.Flags().Changed("type") || !cmd.Flags().Changed("lang") || !cmd.Flags().Changed("journal") {
				if isTerminal() {
					paperType, lang, journal = runInitWizard(paperType, lang, journal,
						cmd.Flags().Changed("type"), cmd.Flags().Changed("lang"), cmd.Flags().Changed("journal"))
				}
			}

			// 枚举校验：无效值直接拒绝，避免写入 manuscript-spec.yaml
			if paperType != "" && paperType != lint.PaperTypeReview && paperType != lint.PaperTypeEmpirical {
				return &paramError{msg: fmt.Sprintf("init: 无效 --type %q（可选 review|empirical）", paperType)}
			}
			if lang != "" && lang != lint.LangZH && lang != lint.LangEN {
				return &paramError{msg: fmt.Sprintf("init: 无效 --lang %q（可选 zh|en）", lang)}
			}
			// 工作目录必须显式设置（FR-LIB-03）：拒绝就地生成污染 CWD
			if err := requireWorkDir(cfg); err != nil {
				return err
			}
			return initWorkdir(cfg.WorkDir, force, refresh, paperType, lang, journal)
		},
	}
	cmd.Flags().Bool("force", false, "覆盖已存在的 .env / .litkit / AGENTS.md")
	cmd.Flags().Bool("refresh", false, "按现有 manuscript-spec.yaml 重新生成 AGENTS.md 撰写段")
	cmd.Flags().String("type", lint.PaperTypeEmpirical, "论文类型：review（综述）| empirical（四段式实证）")
	cmd.Flags().String("lang", lint.LangZH, "撰写语言：zh | en")
	cmd.Flags().String("journal", "", "目标期刊名称（写入 spec，影响引用格式默认值与 checklist）")
	return cmd
}

// initWorkdir 生成 .env / .litkit / AGENTS.md 并初始化 litkit.db。
//
// 文件权限 0600：.env 与 verifier_models.json 可能含 API key，须防其他用户读取。
func initWorkdir(dir string, force, refresh bool, paperType, lang, journal string) error {
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
		spec = lint.SpecForType(paperType, lang)
		spec.Journal = journal
		// 仅 flag 非默认时才覆盖模板 yaml（保留带注释的模板；marshal 输出无注释）
		if spec.PaperType != lint.PaperTypeEmpirical || spec.Lang != lint.LangZH || spec.Journal != "" {
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
	agents := lint.RenderAgentsMD(spec)
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

// isTerminal 检测 stdin 是否为终端（非管道/重定向）。
func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// runInitWizard 交互式初始化向导，逐项询问未显式指定的参数。
// changedX 标记用户是否通过旗标显式传值；已传的跳过询问。
func runInitWizard(typeVal, langVal, journalVal string, changedType, changedLang, changedJournal bool) (string, string, string) {
	scanner := bufio.NewScanner(os.Stdin)

	if !changedType {
		fmt.Println("论文类型？")
		fmt.Println("  1) empirical（四段式实证，默认）")
		fmt.Println("  2) review（综述）")
		fmt.Print("选择 [1/2]: ")
		if scanner.Scan() {
			switch strings.TrimSpace(scanner.Text()) {
			case "2", "review":
				typeVal = lint.PaperTypeReview
			default:
				typeVal = lint.PaperTypeEmpirical
			}
		}
	}

	if !changedLang {
		fmt.Println("撰写语言？")
		fmt.Println("  1) zh（中文，默认）")
		fmt.Println("  2) en（英文）")
		fmt.Print("选择 [1/2]: ")
		if scanner.Scan() {
			switch strings.TrimSpace(scanner.Text()) {
			case "2", "en":
				langVal = lint.LangEN
			default:
				langVal = lint.LangZH
			}
		}
	}

	if !changedJournal {
		fmt.Print("目标期刊（留空跳过）: ")
		if scanner.Scan() {
			journalVal = strings.TrimSpace(scanner.Text())
		}
	}

	return typeVal, langVal, journalVal
}
