package intent

import (
	"testing"

	"github.com/lybxkl/remember-fix/internal/models"
)

func TestDetectKinds(t *testing.T) {
	cases := []struct {
		name string
		in   Input
		kind string
		src  string
	}{
		{"报错截图", Input{Title: "登录按钮点击没反应，控制台报错 500", LastKind: models.KindTodo}, models.KindBug, SourceHeuristic},
		{"截图闪退", Input{Title: "app 闪退", HasImage: true, LastKind: models.KindTodo}, models.KindBug, SourceHeuristic},
		{"异常堆栈", Input{Body: "TypeError: Cannot read properties of undefined\n    at handleClick (app.js:42)"}, models.KindBug, SourceHeuristic},
		{"期望实际不符", Input{Title: "保存后应该跳转，实际停在原地"}, models.KindBug, SourceHeuristic},
		{"数据丢失", Input{Title: "为什么保存后数据丢失了"}, models.KindBug, SourceHeuristic},
		{"界面问题", Input{Title: "今天上线后按钮错位了"}, models.KindBug, SourceHeuristic},
		{"记得开会", Input{Title: "记得明天下午三点开会", LastKind: models.KindBug}, models.KindTodo, SourceHeuristic},
		{"交房租", Input{Title: "明天交房租"}, models.KindTodo, SourceHeuristic},
		{"买牛奶", Input{Title: "买牛奶"}, models.KindTodo, SourceHeuristic},
		{"报销清单", Input{Title: "下周一之前把报销材料交到财务"}, models.KindTodo, SourceHeuristic},
		{"取快递", Input{Title: "周末去取快递"}, models.KindTodo, SourceHeuristic},
		{"编号清单", Input{Title: "1. 买牛奶\n2. 交水电费"}, models.KindTodo, SourceHeuristic},
		{"提交周报", Input{Title: "明天要提交周报"}, models.KindTodo, SourceHeuristic},
		{"改需求", Input{Title: "改一下按钮颜色"}, models.KindTodo, SourceHeuristic},
		{"新增功能", Input{Title: "加个导出功能"}, models.KindTodo, SourceHeuristic},
		{"修复缺陷", Input{Title: "需要修复登录失败的问题"}, models.KindBug, SourceHeuristic},
		{"随手记", Input{Title: "随手记一下", LastKind: models.KindTodo}, models.KindTodo, SourceFallback},
		{"只有名词", Input{Title: "设计稿", LastKind: models.KindBug}, models.KindBug, SourceFallback},
		{"空内容", Input{LastKind: models.KindTodo}, models.KindTodo, SourceFallback},
		{"时间太弱", Input{Title: "明天", LastKind: models.KindBug}, models.KindBug, SourceFallback},
		{"截图但无文字", Input{HasImage: true, SourceApp: "Chrome", LastKind: models.KindTodo}, models.KindTodo, SourceFallback},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Detect(tc.in)
			if got.Kind != tc.kind || got.Source != tc.src {
				t.Fatalf("Detect(%q) = %+v, want kind=%s source=%s", tc.in.Title, got, tc.kind, tc.src)
			}
			if !models.ValidKind(got.Kind) {
				t.Fatalf("invalid kind: %+v", got)
			}
			if got.Confident != (got.Source == SourceHeuristic) {
				t.Fatalf("confident flag mismatch: %+v", got)
			}
			if got.Reason == "" {
				t.Fatalf("reason should not be empty: %+v", got)
			}
		})
	}
}

func TestDetectInvalidLastKindFallsBackToBug(t *testing.T) {
	got := Detect(Input{Title: "随手记一下", LastKind: "nonsense"})
	if got.Kind != models.KindBug || got.Source != SourceFallback {
		t.Fatalf("invalid last kind should fall back to bug: %+v", got)
	}
}
