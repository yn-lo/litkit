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
	"# 所有默认值均可在此调整；AI 经 .litkit/AGENTS.md 获知这些配置\n" +
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

// 文件权限常量（mnd：避免魔法值）。
const (
	initDirPerm  = 0o750
	initFilePerm = 0o600
)

// newInitCmd 构造 `litkit init` 子命令。
//
// 两步式：
//  1. 项目基础设施：.env + .litkit/（litkit.db + AGENTS.md + 共享文件）
//  2. 论文类型目录：.litkit/<type-lang>/ 的 manuscript-spec.yaml
//
// --type + --lang 必填（终端下可交互输入）。
func newInitCmd(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init --type review|empirical|book --lang zh|en [--journal NAME]",
		Short: "初始化项目（.env + .litkit/ 目录）并注册论文类型",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// 工作目录必须显式设置（FR-LIB-03），前置检查
			if err := requireWorkDir(cfg); err != nil {
				return err
			}

			force, _ := cmd.Flags().GetBool("force")
			paperType, _ := cmd.Flags().GetString("type")
			lang, _ := cmd.Flags().GetString("lang")
			journal, _ := cmd.Flags().GetString("journal")

			// --type 和 --lang 必须指定（非终端模式下报错，终端下进入交互向导）
			if !cmd.Flags().Changed("type") || !cmd.Flags().Changed("lang") {
				if isTerminal() {
					paperType, lang, journal = runInitWizard(paperType, lang, journal,
						cmd.Flags().Changed("type"), cmd.Flags().Changed("lang"), cmd.Flags().Changed("journal"))
				} else {
					return &paramError{msg: "init: --type 和 --lang 必须指定（非终端模式下不可省略）"}
				}
			}

			// 枚举校验
			if paperType != "" && !lint.IsValidPaperType(paperType) {
				return &paramError{msg: fmt.Sprintf("init: 无效 --type %q（可选 %s）", paperType, lint.PaperTypesLabel())}
			}
			if lang != "" && lang != lint.LangZH && lang != lint.LangEN {
				return &paramError{msg: fmt.Sprintf("init: 无效 --lang %q（可选 zh|en）", lang)}
			}

			if !cmd.Flags().Changed("journal") && isTerminal() {
				fmt.Print("目标期刊（留空跳过）: ")
				scanner := bufio.NewScanner(os.Stdin)
				if scanner.Scan() {
					journal = strings.TrimSpace(scanner.Text())
				}
			}

			return initWorkdir(cfg.WorkDir, force, paperType, lang, journal)
		},
	}
	cmd.Flags().Bool("force", false, "覆盖已存在的文件")
	cmd.Flags().String("type", lint.PaperTypeEmpirical, "论文类型：review（综述）| empirical（四段式实证）| book（书籍）")
	cmd.Flags().String("lang", lint.LangZH, "撰写语言：zh | en")
	cmd.Flags().String("journal", "", "目标期刊名称（写入 spec，影响引用格式默认值）")
	return cmd
}

// initWorkdir 两步式初始化。
func initWorkdir(dir string, force bool, paperType, lang, journal string) error {
	created := []string{}

	// 第一步：项目基础设施（幂等）
	infraCreated, err := ensureProjectInfra(dir, force)
	if err != nil {
		return err
	}
	created = append(created, infraCreated...)

	// 第二步：论文类型注册
	typeCreated, err := ensurePaperType(dir, force, paperType, lang, journal)
	if err != nil {
		return err
	}
	created = append(created, typeCreated...)

	return printJSON(map[string]any{
		"workDir": dir,
		"created": created,
	})
}

// ensureProjectInfra 确保项目基础设施存在（.env + .litkit/ 下 litkit.db + AGENTS.md + 共享文件）。
func ensureProjectInfra(dir string, force bool) ([]string, error) {
	created := []string{}

	// .env
	if ok, err := writeIfAbsent(filepath.Join(dir, ".env"), envTemplate, force); err != nil {
		return nil, err
	} else if ok {
		created = append(created, ".env")
	}

	// .litkit/ 共享文件（verifier_models.json）
	infraFiles, err := lint.InitProjectInfra(dir, force)
	if err != nil {
		return nil, err
	}
	created = append(created, infraFiles...)

	// AGENTS.md（项目级通用信息，从 embed 模板复制到 .litkit/）
	rootAgents, err := lint.RootAgentsContent()
	if err != nil {
		return nil, err
	}
	if ok, err := writeIfAbsent(filepath.Join(dir, lint.LitkitDir, "AGENTS.md"), rootAgents, force); err != nil {
		return nil, err
	} else if ok {
		created = append(created, filepath.Join(lint.LitkitDir, "AGENTS.md"))
	}

	// .litkit/skills/ Agent Skills（用户按自己 AI 工具手动复制到对应 skills 路径）
	skillFiles, err := lint.InitSkills(dir, force)
	if err != nil {
		return nil, err
	}
	created = append(created, skillFiles...)

	// litkit.db（幂等：已存在则跳过建表）
	// loadDeps（PersistentPreRunE）可能已提前调用 storage.Open 创建了 db 文件，
	// 此处再次 Open 确保测试路径（直接调用 initWorkdir）也能创建 db。
	dbPath := storage.DBPath(dir)
	store, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("init: storage open: %w", err)
	}
	_ = store.Close()
	created = append(created, filepath.Join(".litkit", storage.DefaultDBName))

	return created, nil
}

// ensurePaperType 确保论文类型目录存在（.litkit/<type-lang>/ 的 manuscript-spec.yaml）。
func ensurePaperType(dir string, force bool, paperType, lang, journal string) ([]string, error) {
	created := []string{}

	// 复制模板 yaml
	yamlCreated, err := lint.InitPaperType(dir, paperType, lang, force)
	if err != nil {
		return nil, err
	}
	created = append(created, yamlCreated...)

	// 加载 spec（优先从 yaml，否则从模板）
	specPath := lint.SpecPath(dir, paperType, lang)
	spec, err := lint.LoadSpec(specPath)
	if err != nil {
		// yaml 刚被复制，不应失败
		spec = lint.SpecForType(paperType, lang)
	}

	// journal 覆盖
	if journal != "" {
		spec.Journal = journal
		// 写回 yaml
		if err := lint.WriteSpec(specPath, spec); err != nil {
			return nil, err
		}
	}

	return created, nil
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
func runInitWizard(typeVal, langVal, journalVal string, changedType, changedLang, changedJournal bool) (string, string, string) {
	scanner := bufio.NewScanner(os.Stdin)

	if !changedType {
		fmt.Println("论文类型？")
		fmt.Println("  1) empirical（四段式实证，默认）")
		fmt.Println("  2) review（综述）")
		fmt.Println("  3) book（书籍，中文编校细则）")
		fmt.Print("选择 [1/2/3]: ")
		if scanner.Scan() {
			switch strings.TrimSpace(scanner.Text()) {
			case "2", "review":
				typeVal = lint.PaperTypeReview
			case "3", "book":
				typeVal = lint.PaperTypeBook
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
