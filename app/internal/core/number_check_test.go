package core

import (
	"testing"
)

func TestCheckNumericConsistency_AllMatch(t *testing.T) {
	sentence := "实验结果显示准确率达 95.2%，p < 0.01"
	abstract := "该模型准确率 95.2%，显著性水平 p < 0.01"
	issues := CheckNumericConsistency(sentence, abstract)
	if len(issues) != 0 {
		t.Fatalf("所有数字均匹配时应返回空，got %d", len(issues))
	}
}

func TestCheckNumericConsistency_NumberNotInAbstract(t *testing.T) {
	sentence := "实验结果显示准确率达 95.2%"
	abstract := "该模型准确率 90.0%"
	issues := CheckNumericConsistency(sentence, abstract)
	if len(issues) != 1 {
		t.Fatalf("应标记 1 个不匹配数字，got %d", len(issues))
	}
	if issues[0].Number != "95.2%" {
		t.Fatalf("应标记 95.2%%，got %q", issues[0].Number)
	}
	if issues[0].InAbstract {
		t.Fatal("InAbstract 应为 false")
	}
}

func TestCheckNumericConsistency_MultipleNumbers(t *testing.T) {
	sentence := "实验纳入 1000 名患者，年龄 45.2±3.1 岁，p=0.03"
	abstract := "纳入 1000 名患者，平均年龄 45 岁"
	issues := CheckNumericConsistency(sentence, abstract)
	if len(issues) == 0 {
		t.Fatal("应标记不匹配的数字")
	}
	// p=0.03 和 3.1 不在摘要中
	found := false
	for _, iss := range issues {
		if iss.Number == "p=0.03" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("应标记 p=0.03，got %v", issues)
	}
}

func TestCheckNumericConsistency_NoNumbers(t *testing.T) {
	sentence := "该方法在实验中表现良好"
	abstract := "该方法在多个实验中表现优异"
	issues := CheckNumericConsistency(sentence, abstract)
	if len(issues) != 0 {
		t.Fatalf("无数字时应返回空，got %d", len(issues))
	}
}

func TestCheckNumericConsistency_PValue(t *testing.T) {
	// ponytail: 简单后缀匹配 p < 0.05 与 p=0.05 可能不匹配同一抽象写法
	// 标定期根据实际数据调整归一化策略
	sentence := "结果具有统计学意义（p < 0.05）"
	abstract := "p < 0.05 表示显著"
	issues := CheckNumericConsistency(sentence, abstract)
	if len(issues) != 0 {
		t.Fatalf("p 值匹配时应返回空，got %d issues: %v", len(issues), issues)
	}
}

func TestCheckNumericConsistency_Year(t *testing.T) {
	sentence := "该研究发表于 2024 年"
	abstract := "2024 年的研究"
	issues := CheckNumericConsistency(sentence, abstract)
	if len(issues) != 0 {
		t.Fatalf("年份匹配时应返回空，got %d", len(issues))
	}
}

func TestCheckNumericConsistency_YearNotInAbstract(t *testing.T) {
	sentence := "该研究发表于 2024 年"
	abstract := "近年来的研究"
	issues := CheckNumericConsistency(sentence, abstract)
	if len(issues) != 1 {
		t.Fatalf("应标记不匹配年份，got %d", len(issues))
	}
	if issues[0].Number != "2024" {
		t.Fatalf("应标记 2024，got %q", issues[0].Number)
	}
}

func TestCheckNumericConsistency_SingleDigitIgnored(t *testing.T) {
	sentence := "采用 2 种方法进行分析，准确率 95%"
	abstract := "准确率 95%"
	issues := CheckNumericConsistency(sentence, abstract)
	// 单个数字 "2" 被忽略，95% 应匹配
	if len(issues) != 0 {
		t.Fatalf("个位数应忽略，95%% 匹配时无问题，got %d", len(issues))
	}
}

func TestCheckNumericConsistency_StatExpr(t *testing.T) {
	sentence := "F(1,28)=4.52, p=0.042"
	abstract := "F(1,28)=4.52, p=0.042"
	issues := CheckNumericConsistency(sentence, abstract)
	if len(issues) != 0 {
		t.Fatalf("统计量表达式匹配时应返回空，got %d", len(issues))
	}
}

func TestExtractNumbers(t *testing.T) {
	text := "准确率 95.2%，p < 0.01，F(1,28)=4.52，纳入 1000 人"
	nums := extractNumbers(text)
	if len(nums) == 0 {
		t.Fatal("应提取数字")
	}
	hasP := false
	hasF := false
	hasPercent := false
	hasThousand := false
	for _, n := range nums {
		if n == "95.2%" {
			hasPercent = true
		}
		if n == "1000" {
			hasThousand = true
		}
		if n == "p < 0.01" {
			hasP = true
		}
		if n == "F(1,28)=4.52" {
			hasF = true
		}
	}
	if !hasPercent {
		t.Fatal("应提取 95.2%")
	}
	if !hasThousand {
		t.Fatal("应提取 1000")
	}
	if !hasP {
		t.Fatal("应提取 p < 0.01")
	}
	if !hasF {
		t.Fatal("应提取 F(1,28)=4.52")
	}
}
