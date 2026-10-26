// Package intent 用本地规则猜测一条随手记录更像 Bug 还是待办。
// 规则只负责给出低成本的初判；标为 fallback 时沿用用户上次使用的类型，交给 AI 后续纠正。
package intent

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/lybxkl/remember-fix/internal/models"
)

const (
	SourceHeuristic = "heuristic"
	SourceFallback  = "fallback"
)

type Input struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	SourceApp string `json:"sourceApp"`
	HasImage  bool   `json:"hasImage"`
	LastKind  string `json:"lastKind"`
}

type Result struct {
	Kind       string  `json:"kind"`
	Confidence float64 `json:"confidence"`
	Source     string  `json:"source"`
	Confident  bool    `json:"confident"`
	Reason     string  `json:"reason"`
}

var (
	todoStrongWords = []string{
		"记得", "别忘了", "别忘记", "提醒我", "待办", "要做", "截止", "deadline", "ddl",
		"预约", "缴费", "交费", "还款", "续费", "报名", "打卡", "开会", "会议", "面试",
		"体检", "生日", "纪念日", "取快递", "寄快递", "买菜", "购物", "订票", "机票",
		"车票", "酒店", "日程", "安排", "计划", "准备", "提交", "交付", "汇报", "复习",
		"学习", "阅读", "联系", "回复", "确认", "整理", "报销", "搬家", "还书", "房租",
		"水电费", "物业费", "周报", "日报", "晨会", "团建", "接送", "转账", "充值",
	}
	todoTaskWords = []string{
		"改一下", "改下", "改成", "换成", "加个", "加一个", "新增", "支持", "实现",
		"重构", "拆分", "优化", "调整", "替换", "接入", "补一个", "去掉", "删掉",
	}
	todoWeakWords  = []string{"升级", "发布"}
	bugStrongWords = []string{
		"崩溃", "闪退", "白屏", "黑屏", "卡死", "死机", "报错", "异常", "错误", "失败",
		"无法", "不能", "打不开", "没反应", "无响应", "不生效", "失效", "数据丢失",
		"不同步", "内存泄漏", "复现", "堆栈", "日志", "bug", "exception", "traceback",
		"typeerror", "referenceerror", "syntaxerror", "nullpointer", "panic", "crash",
		"freeze", "failed", "broken", "not working",
	}
	bugUIWords = []string{
		"样式", "界面", "按钮", "弹窗", "布局", "重叠", "错位", "遮挡", "乱码",
		"卡顿", "性能", "兼容", "闪烁", "错乱", "显示异常", "不显示", "显示不全",
	}
	todoStartRe = regexp.MustCompile(`^(?:请|帮我|帮忙|记得|别忘了|提醒我|今天|明天|后天|下周|周末|稍后|待会)?\s*[^\n。！？]{0,8}(买|交|约|打|问|拿|寄|订|取|送|做|写|看|读|学|找|联系|回复|确认|准备|整理|提交|安排|预约|缴费|充值)`)
	todoTimeRe  = regexp.MustCompile(`(今天|明天|后天|大后天|今晚|明早|下周|下个月|周末|月底|月初|周[一二三四五六日天]|星期[一二三四五六日天]|[0-9]{1,2}月[0-9]{1,2}[日号]?|[0-9]{1,2}[日号]|(?:[01]?[0-9]|2[0-3])[:：][0-5][0-9]|[0-9]{1,2}点)`)
	bugExpectRe = regexp.MustCompile(`(应该|本来|期望|按理说|正常情况).{0,24}(却|但|结果|实际)`)
	bugStackRe  = regexp.MustCompile(`(?m)(^|\s)at\s+[\w.$]+\([^)]*:\d+\)|Traceback \(most recent call last\)|panic:|goroutine \d+ \[`)
	bugAskRe    = regexp.MustCompile(`为什么|怎么(会|回事)|咋(会|回事)|是不是`)
	todoListRe  = regexp.MustCompile(`(?m)^\s*(?:[-*•]\s*|\d+[.、)]\s*)(?:\[[ xX]\]\s*)?\S`)
)

// Detect 返回本地初判。没有足够线索时 Source=fallback，Kind 为上次类型（默认 bug）。
func Detect(in Input) Result {
	lastKind := in.LastKind
	if !models.ValidKind(lastKind) {
		lastKind = models.KindBug
	}
	text := strings.TrimSpace(in.Title)
	if body := strings.TrimSpace(in.Body); body != "" {
		if text != "" {
			text += "\n"
		}
		text += body
	}
	if utf8.RuneCountInString(text) == 0 {
		return fallback(lastKind, "还没有内容，先沿用上次类型")
	}

	lower := strings.ToLower(text)
	var bugScore, todoScore float64
	var bugHit, todoHit string
	addBug := func(score float64, hit string) {
		bugScore += score
		if bugHit == "" {
			bugHit = hit
		}
	}
	addTodo := func(score float64, hit string) {
		todoScore += score
		if todoHit == "" {
			todoHit = hit
		}
	}

	if strings.Contains(lower, "todo") || strings.Contains(lower, "[ ]") {
		addTodo(4, "todo")
	}
	for _, w := range todoStrongWords {
		if strings.Contains(lower, w) {
			addTodo(3, w)
		}
	}
	for _, w := range todoTaskWords {
		if strings.Contains(lower, w) {
			addTodo(3, w)
		}
	}
	for _, w := range todoWeakWords {
		if strings.Contains(lower, w) {
			addTodo(1.5, w)
		}
	}
	if m := todoStartRe.FindStringSubmatch(lower); m != nil {
		addTodo(2, m[1])
	}
	if matches := todoTimeRe.FindAllString(lower, 4); len(matches) > 0 {
		addTodo(1.5, matches[0])
		if len(matches) > 1 {
			addTodo(0.5*float64(len(matches)-1), matches[1])
		}
	}
	if n := len(todoListRe.FindAllString(text, 5)); n >= 2 {
		addTodo(2.5, "清单")
	}

	for _, w := range bugStrongWords {
		if strings.Contains(lower, w) {
			addBug(3.5, w)
		}
	}
	for _, w := range bugUIWords {
		if strings.Contains(lower, w) {
			addBug(2, w)
		}
	}
	if strings.Contains(lower, "修复") {
		addBug(2.5, "修复")
	}
	if bugExpectRe.MatchString(lower) {
		addBug(3, "期望/实际不符")
	}
	if bugStackRe.MatchString(text) {
		addBug(4, "堆栈")
	}
	if bugAskRe.MatchString(lower) {
		addBug(1.5, "疑问")
	}
	if in.HasImage {
		addBug(1.2, "截图")
	}
	if strings.TrimSpace(in.SourceApp) != "" {
		addBug(0.6, "来源应用")
	}

	if bugScore == 0 && todoScore == 0 {
		return fallback(lastKind, "没发现明确线索，沿用上次类型")
	}
	if bugScore > todoScore {
		if bugScore-todoScore < 1 {
			return fallback(lastKind, "Bug 和待办线索接近，沿用上次类型")
		}
		conf := scoreConfidence(bugScore, bugScore-todoScore)
		if conf < 0.65 {
			return fallback(lastKind, "线索偏弱，沿用上次类型")
		}
		return Result{Kind: models.KindBug, Confidence: conf, Source: SourceHeuristic, Confident: true,
			Reason: "识别到 Bug 线索「" + bugHit + "」"}
	}
	if todoScore-bugScore < 1 {
		return fallback(lastKind, "Bug 和待办线索接近，沿用上次类型")
	}
	conf := scoreConfidence(todoScore, todoScore-bugScore)
	if conf < 0.65 {
		return fallback(lastKind, "线索偏弱，沿用上次类型")
	}
	return Result{Kind: models.KindTodo, Confidence: conf, Source: SourceHeuristic, Confident: true,
		Reason: "识别到待办线索「" + todoHit + "」"}
}

func scoreConfidence(winner, margin float64) float64 {
	if winner > 4 {
		winner = 4
	}
	if margin > 4 {
		margin = 4
	}
	conf := 0.45 + 0.07*winner + 0.05*margin
	if conf > 0.96 {
		conf = 0.96
	}
	return float64(int(conf*100+0.5)) / 100
}

func fallback(kind, reason string) Result {
	return Result{Kind: kind, Confidence: 0.35, Source: SourceFallback, Confident: false, Reason: reason}
}
