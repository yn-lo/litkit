package main

import (
	"testing"

	"litkit/internal/lint"
)

// TestBuildRulesOutput 规则注册表投影应包含全部规则与关键字段。
func TestBuildRulesOutput(t *testing.T) {
	out := buildRulesOutput()
	ids := map[string]bool{}
	for _, r := range out {
		if r.ID == "" || r.Name == "" || r.Category == "" {
			t.Errorf("规则字段不应为空：%+v", r)
		}
		ids[r.ID] = true
	}
	// 关键规则（含本次新增的 R3.3/R3.4）必须可查
	for _, id := range []string{"R1.3", "R3.3", "R3.4", "R2.1", "R7.2"} {
		if !ids[id] {
			t.Errorf("规则 %s 应在注册表输出中", id)
		}
	}
}

// TestRulesCmd 命令应可执行且不依赖工作目录。
func TestRulesCmd(t *testing.T) {
	cmd := newRulesCmd()
	if err := cmd.Execute(); err != nil {
		t.Fatalf("rules 命令执行失败: %v", err)
	}
}

// TestAllRules_registryIntegrity 注册表完整性：ID 唯一、字段非空、Check 必填。
func TestAllRules_registryIntegrity(t *testing.T) {
	rules := lint.AllRules()
	seen := map[string]bool{}
	for _, r := range rules {
		if r.ID == "" || r.Name == "" {
			t.Errorf("规则缺 ID/名称：%+v", r)
		}
		if seen[r.ID] {
			t.Errorf("规则 ID 重复：%s", r.ID)
		}
		seen[r.ID] = true
		if r.Check == nil {
			t.Errorf("规则 %s 缺 Check", r.ID)
		}
		if len(r.Langs) == 0 {
			t.Errorf("规则 %s 缺 Langs", r.ID)
		}
	}
}
