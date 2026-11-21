package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"litkit/internal/config"
	"litkit/internal/lint"
	"litkit/internal/storage"
)

// envTemplate 工作目录 .env 模板（litkit init 生成，FR-CONFIG-03）。
// 覆盖 app/.env.example 的全部配置项；TestEnvTemplate_matchesEnvExample 保证两者不漂移。
const envTemplate = "# litkit 工作目录配置（由 litkit init 生成，FR-CONFIG-03）\n" +
	"# 所有默认值均可在此调整；AI 经 .litkit/AGENTS.md 获知这些配置\n" +
	"# 完整清单与逐项注释见 app/.env.example\n" +
	"\n" +
	"# --- 工作目录与配置 ---\n" +
	"# 工作目录（必填：未设置时 litkit 拒绝执行，FR-LIB-03）\n" +
	"LITKIT_WORK_DIR=.\n" +
	"# 显式 .env 路径（默认按 WORK_DIR > CWD 向上溯源 查找）\n" +
	"# LITKIT_ENV_FILE=\n" +
	"# 默认写作语言模式（zh | en），作为 init/manuscript/verify 的 --lang 默认值\n" +
	"# LITKIT_LANG=zh\n" +
	"\n" +
	"# --- HTTP 客户端 ---\n" +
	"# 单请求超时（毫秒），默认 15000\n" +
	"# LITKIT_HTTP_TIMEOUT_MS=15000\n" +
	"# 429/5xx 重试次数，默认 2\n" +
	"# LITKIT_HTTP_RETRIES=2\n" +
	"# 显式代理（http/https/socks5，如本机 Clash：http://127.0.0.1:7890）\n" +
	"# 设置后所有外呼（检索源/元数据/全文/LLM 评分）统一走代理\n" +
	"# LITKIT_PROXY_URL=\n" +
	"\n" +
	"# --- 检索 ---\n" +
	"# 每源默认检索条数，默认 5\n" +
	"# LITKIT_DEFAULT_MAX_RESULTS=5\n" +
	"# 默认检索时间范围（最近 N 年），默认 3；0=不限\n" +
	"# LITKIT_DEFAULT_RECENT_YEARS=3\n" +
	"# 默认检索等级：tiab（题目+摘要+关键词，默认）| full（全文）\n" +
	"# LITKIT_DEFAULT_SEARCH_MODE=tiab\n" +
	"# 整体检索超时（毫秒），默认 60000\n" +
	"# LITKIT_SEARCH_TIMEOUT_MS=60000\n" +
	"# 默认检索源白名单（逗号分隔；留空=默认参与检索的源，可用源见 litkit sources）\n" +
	"# 注：biorxiv/medrxiv 无关键词检索端点，默认不参与，需 -s 显式指定\n" +
	"# LITKIT_SEARCH_SOURCES=\n" +
	"# 默认排除源（逗号分隔；仅作用于未显式 -s 的检索）\n" +
	"# LITKIT_SEARCH_EXCLUDE_SOURCES=\n" +
	"# 可选：Semantic Scholar API key（配置后提速，避免共享池 429）\n" +
	"# LITKIT_SEMANTIC_SCHOLAR_API_KEY=\n" +
	"# 可选：PubMed API key（无 key 3 req/s；有 key 10 req/s）\n" +
	"# LITKIT_PUBMED_API_KEY=\n" +
	"# 二期源激活（IEEE/ACM，本期不实现）\n" +
	"# LITKIT_IEEE_API_KEY=\n" +
	"# LITKIT_ACM_API_KEY=\n" +
	"\n" +
	"# --- LLM 引用评分（可选）---\n" +
	"# 推荐直接在 .litkit/verifier_models.json 每模型 api_key/base_url 配置（该副本勿提交 git）\n" +
	"# 启用条件：models[].enabled=true 且有 api_key 即自动生效，无需总开关\n" +
	"# 以下为 env 回落方式（JSON 未填时生效；按模型变量优先于全局）\n" +
	"# LITKIT_LLM_API_KEY=sk-xxx\n" +
	"# LITKIT_LLM_BASE_URL=https://api.openai.com/v1\n" +
	"# 按模型命名约定：模型 id 大写、非字母数字转下划线（如 deepseek-chat → DEEPSEEK_CHAT）\n" +
	"# LITKIT_LLM_API_KEY_DEEPSEEK_CHAT=sk-xxx\n" +
	"# LITKIT_LLM_BASE_URL_DEEPSEEK_CHAT=https://api.deepseek.com/v1\n" +
	"# LITKIT_LLM_API_KEY_QWEN_PLUS=sk-xxx\n" +
	"# LITKIT_LLM_BASE_URL_QWEN_PLUS=https://dashscope.aliyuncs.com/compatible-mode/v1\n" +
	"# 单次评分超时（毫秒），默认 30000\n" +
	"# LITKIT_LLM_TIMEOUT_MS=30000\n" +
	"\n" +
	"# --- 全文获取（可选）---\n" +
	"# Unpaywall OA 解析必需（https://unpaywall.org/products/api 免费申请）\n" +
	"# LITKIT_UNPAYWALL_EMAIL=\n" +
	"# Sci-Hub 兜底镜像（默认 https://sci-hub.se）\n" +
	"# LITKIT_SCI_HUB_URL=https://sci-hub.se\n" +
	"# 全文 PDF 落盘目录（默认 <LITKIT_WORK_DIR>/downloads；相对路径按 .env 目录解析）\n" +
	"# LITKIT_FETCH_DOWNLOAD_DIR=\n"

// gitignoreBlock litkit 在项目 .gitignore 中维护的受管区块（幂等追加）。
const gitignoreBlock = "# >>> litkit（由 `litkit init` 生成，勿删） >>>\n" +
	".env\n" +
	".litkit/litkit.db\n" +
	"*.db-shm\n" +
	"*.db-wal\n" +
	".litkit/verifier_models.json\n" +
	"outputs/\n" +
	"downloads/\n" +
	"# <<< litkit <<<\n"

// gitignoreMarker 受管区块起始标记（存在即视为已写入，避免重复追加）。
const gitignoreMarker = "# >>> litkit（由 `litkit init` 生成，勿删） >>>"

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
		Use:   "init --type review|empirical|qualitative|book|proposal --lang zh|en [--journal NAME]",
		Short: "初始化项目（.env + .litkit/ 目录）并注册论文类型",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// 工作目录必须显式设置（FR-LIB-03）：终端下可交互补问，非终端直接报错
			if cfg.WorkDir == "" {
				if !isTerminal() {
					return errNoWorkDir
				}
				cwd, err := os.Getwd()
				if err != nil {
					return errNoWorkDir
				}
				cfg.WorkDir = promptWorkDir(os.Stdin, cwd)
			}
			if err := requireWorkDir(cfg); err != nil {
				return err
			}

			force, _ := cmd.Flags().GetBool("force")
			paperType, _ := cmd.Flags().GetString("type")
			lang, _ := cmd.Flags().GetString("lang")
			journal, _ := cmd.Flags().GetString("journal")

			// --type 和 --lang 必须指定（非终端模式下报错，终端下进入交互向导）
			wizardRan := false
			if !cmd.Flags().Changed("type") || !cmd.Flags().Changed("lang") {
				if isTerminal() {
					paperType, lang, journal = runInitWizard(paperType, lang, journal,
						cmd.Flags().Changed("type"), cmd.Flags().Changed("lang"), cmd.Flags().Changed("journal"))
					wizardRan = true
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

			// 向导已问过期刊；仅在跳过向导时单独补问，避免重复提问
			if !wizardRan && !cmd.Flags().Changed("journal") && isTerminal() {
				fmt.Print("目标期刊（留空跳过）: ")
				scanner := bufio.NewScanner(os.Stdin)
				if scanner.Scan() {
					journal = strings.TrimSpace(scanner.Text())
				}
			}

			return initWorkdir(cfg.WorkDir, force, paperType, lang, journal)
		},
	}
	cmd.Flags().Bool("force", false, "覆盖已生成的模板文件（.env / verifier_models.json 除外，避免丢失密钥）")
	cmd.Flags().String("type", lint.PaperTypeEmpirical, fmt.Sprintf("论文类型：%s", lint.PaperTypesLabel()))
	cmd.Flags().String("lang", defaultLang(cfg), "撰写语言：zh | en（默认 LITKIT_LANG）")
	cmd.Flags().String("journal", "", "目标期刊名称（写入 spec，影响引用格式默认值）")
	return cmd
}

// initWorkdir 两步式初始化。
func initWorkdir(dir string, force bool, paperType, lang, journal string) error {
	created := []string{}

	// 允许初始化到尚不存在的目录（向导中输入新路径时常见）
	if err := os.MkdirAll(dir, initDirPerm); err != nil {
		return fmt.Errorf("init: 创建目录 %s 失败: %w", dir, err)
	}

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

	// .env 永不覆盖（可能含用户 API key）：force 只刷新模板生成的其余文件
	envPath := filepath.Join(dir, ".env")
	if ok, err := writeIfAbsent(envPath, envTemplate, false); err != nil {
		return nil, err
	} else if ok {
		created = append(created, ".env")
	} else if added, err := ensureEnvWorkDir(envPath); err != nil {
		return nil, err
	} else if added {
		// 已有 .env（可能是其他工具的配置）：只补 LITKIT_WORK_DIR，不覆盖原内容
		created = append(created, ".env（补充 LITKIT_WORK_DIR）")
	}

	// .gitignore 受管区块：避免 .env（密钥）/ 本地库 / 产物被提交
	if ok, err := ensureGitignore(dir); err != nil {
		return nil, err
	} else if ok {
		created = append(created, ".gitignore")
	}

	// .litkit/ 共享文件（verifier_models.json；已存在时永不覆盖，可能含 API key）
	infraFiles, err := lint.InitProjectInfra(dir)
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

	// .agents/skills/ Agent Skills（仓库级标准位置，AI 工具自动扫描发现，无需手动复制）
	skillFiles, err := lint.InitSkills(dir, force)
	if err != nil {
		return nil, err
	}
	created = append(created, skillFiles...)

	// litkit.db（幂等：已存在则跳过建表）
	// 依赖注入已改为按需开库，init 自身负责首次建库（测试直接调用 initWorkdir 也走这里）。
	dbPath := storage.DBPath(dir)
	dbExisted := fileExists(dbPath)
	store, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("init: storage open: %w", err)
	}
	_ = store.Close()
	if !dbExisted {
		created = append(created, filepath.Join(".litkit", storage.DefaultDBName))
	}

	return created, nil
}

// ensureGitignore 在项目 .gitignore 追加 litkit 受管区块；已存在区块时跳过。
// 保留用户原有内容（可能是其他工具的忽略规则）。返回是否写入。
func ensureGitignore(dir string) (bool, error) {
	path := filepath.Join(dir, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("init: read %s: %w", path, err)
	}
	if strings.Contains(string(data), gitignoreMarker) {
		return false, nil
	}
	prefix := ""
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		prefix = "\n"
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, initFilePerm)
	if err != nil {
		return false, fmt.Errorf("init: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(prefix + gitignoreBlock); err != nil {
		return false, fmt.Errorf("init: append %s: %w", path, err)
	}
	return true, nil
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

	// journal 覆盖（只替换 journal 行，保留模板顶部撰写规定注释）
	if journal != "" {
		if err := lint.SetSpecJournal(lint.SpecPath(dir, paperType, lang), journal); err != nil {
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

// ensureEnvWorkDir 在已有 .env 中补充缺失的 LITKIT_WORK_DIR（保留原内容）。
// 项目可能已有其他工具的 .env；此时不能覆盖，但必须补上工作目录，
// 否则 init 成功后续命令仍会报未设置 LITKIT_WORK_DIR。
func ensureEnvWorkDir(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("init: read %s: %w", path, err)
	}
	for _, ln := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		t := strings.TrimPrefix(strings.TrimSpace(ln), "\uFEFF") // 兼容 UTF-8 BOM
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if k, v, ok := strings.Cut(t, "="); ok && strings.TrimSpace(k) == "LITKIT_WORK_DIR" {
			if strings.TrimSpace(v) != "" {
				return false, nil
			}
			// 空值视为缺失，继续补一行（后出现的键覆盖前者）
		}
	}
	prefix := ""
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		prefix = "\n"
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, initFilePerm)
	if err != nil {
		return false, fmt.Errorf("init: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(prefix + "LITKIT_WORK_DIR=.\n"); err != nil {
		return false, fmt.Errorf("init: append %s: %w", path, err)
	}
	return true, nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// isTerminal 检测 stdin 是否为交互终端（管道/重定向/NUL 均为 false）。
func isTerminal() bool {
	return isatty.IsTerminal(os.Stdin.Fd())
}

// promptWorkDir 交互式询问工作目录；留空使用当前目录。
func promptWorkDir(in io.Reader, cwd string) string {
	fmt.Printf("工作目录（回车使用当前目录 %s）: ", cwd)
	scanner := bufio.NewScanner(in)
	if scanner.Scan() {
		if v := strings.TrimSpace(scanner.Text()); v != "" {
			return v
		}
	}
	return cwd
}

// runInitWizard 交互式初始化向导，逐项询问未显式指定的参数。
func runInitWizard(typeVal, langVal, journalVal string, changedType, changedLang, changedJournal bool) (string, string, string) {
	scanner := bufio.NewScanner(os.Stdin)

	if !changedType {
		fmt.Println("论文类型？")
		fmt.Println("  1) empirical（四段式实证，默认）")
		fmt.Println("  2) review（综述）")
		fmt.Println("  3) book（书籍，中文编校细则）")
		fmt.Println("  4) qualitative（质性研究，主题呈现）")
		fmt.Println("  5) proposal（学术标书/课题申请书，中文）")
		fmt.Print("选择 [1/2/3/4/5]: ")
		if scanner.Scan() {
			switch strings.TrimSpace(scanner.Text()) {
			case "2", "review":
				typeVal = lint.PaperTypeReview
			case "3", "book":
				typeVal = lint.PaperTypeBook
			case "4", "qualitative":
				typeVal = lint.PaperTypeQualitative
			case "5", "proposal":
				typeVal = lint.PaperTypeProposal
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
