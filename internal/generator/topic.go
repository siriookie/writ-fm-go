package generator

import (
	"math/rand"
	"regexp"
	"strings"
)

var formattedHeadlineTitleRE = regexp.MustCompile(`(?m)^\s*\d+\.\s*\[[^\]]+\]\s*([^\r\n]+)\s*$`)

// InterviewGuest is a fictional/composite guest profile for interview segments.
type InterviewGuest struct {
	Name    string
	Context string
}

// TopicPools organizes candidate topics by show topic focus.
var TopicPools = map[string][]string{
	"philosophy": {
		"凌晨三点的大脑，为什么在黑暗里更容易想得太多",
		"大众媒介里的亲密幻觉，我们为何会对陌生声音产生依赖",
		"记忆考古学，一首歌如何把被埋住的过去重新挖出来",
		"灵魂的候车室，我们栖居过的那些过渡地带",
		"失眠的民众，今晚还有谁和你一起醒着",
	},
	"music_history": {
		"B 面时代，被随手塞过去的歌为什么后来成了经典",
		"地理如何塑造声音，哪些城市发明了新的音乐语法",
		"录音室也是乐器，那些塑造了时代音色的房间",
		"海盗电台的黄金年代，空气中的亡命之徒与自由声音",
		"DJ 作为策展人，选择与排序为什么本身就是创作",
	},
	"current_events": {
		"这组新闻背后的责任链：谁在行动，谁在解释，谁在承担后果",
		"突发事件里的信息真空：确认、推断和政治叙事如何抢夺同一个现场",
		"从一条新闻看制度反应：执法、媒体、公众和利益方各自做了什么",
		"当新闻进入全球供应链：企业、市场和政策压力如何重新分配风险",
		"今天的公共议题里，哪些问题被标题盖住了，哪些证据还不够",
		"从最新 RSS 材料拆一条事实链：时间线、利益关系和下一步变量",
	},
	"culture": {
		"咖啡馆为何会成为第三空间，陌生人如何慢慢变成熟面孔",
		"夜班劳动者，那套维持城市运转却常常不可见的系统",
		"凌晨两点的简餐店，为什么总像一个临时告解室",
		"书店作为避难所，纸本阅读的安静抵抗",
		"混音磁带的艺术，播放列表为什么像没寄出的情书",
	},
	"soul_music": {
		"一首歌为什么会有灵魂，Soul 从来不只是曲风分类",
		"支撑每一道 groove 的福音根系",
		"放克也是一种哲学，Parliament 为什么像一艘宇宙母船",
		"Erykah Badu 与她的气场教会",
		"迪斯科的死亡与复活，谁曾关掉舞池，又是谁重新点亮它",
	},
	"night_philosophy": {
		"黑暗知道而白天不知道的事",
		"梦像潜意识经营的一座深夜电台",
		"凌晨四点的告白，为什么真话总在黑暗里更容易说出口",
		"失眠像一种不被欢迎的清醒",
		"为什么很多创造力偏偏在午夜之后出现",
	},
	"listeners": {
		"来自这个频率的来信，我们一封一封回",
		"那些改变过你人生的歌，听众自己的故事",
		"黑暗中的提问，你一直想知道却没地方问出口的事",
		"收件箱里的点歌、坦白与小型忏悔",
		"你正从哪里收听，这个听众地图到底长什么样",
	},
}

// InterviewGuests mirrors the Python generator's fictional guest pool.
var InterviewGuests = []InterviewGuest{
	{Name: "一位来自底特律、已经退休的唱片店老板", Context: "在社区里整理和推荐黑胶唱片将近四十年"},
	{Name: "一位参与过传奇录音现场的录音工程师", Context: "亲眼看见过历史被一圈磁带录下来的瞬间"},
	{Name: "一位研究广播史的学者", Context: "长期研究海盗电台与社区广播的黄金年代"},
	{Name: "一位大学馆藏里的爵士档案员", Context: "正在整理一个世纪里被遗忘的录音"},
	{Name: "一位每晚都在收听我们的夜班护士", Context: "知道医院里那些不写在制度里的秘密配乐"},
}

// SelectTopic picks a topic for the given show focus. If the focus is unknown,
// it falls back to the combined pool of all topics.
func SelectTopic(topicFocus string) string {
	return selectTopicWithRand(topicFocus, rand.Intn)
}

func selectTopicWithRand(topicFocus string, randIntn func(int) int) string {
	pool := TopicPools[topicFocus]
	if len(pool) == 0 {
		pool = allTopics()
	}
	if len(pool) == 0 {
		return ""
	}
	return pool[randIntn(len(pool))]
}

func deriveNewsAnalysisTopic(headlines string) string {
	titles := extractFormattedHeadlineTitles(headlines, 3)
	if len(titles) == 0 {
		return "根据今日 RSS 材料，自行判断最值得普通人搞清楚的一条新闻主线"
	}
	if len(titles) == 1 {
		return "从「" + titles[0] + "」讲清这条新闻背后的关键问题"
	}
	return "根据今日 RSS 材料，自行判断最值得分析的新闻主线；优先比较「" + titles[0] + "」和「" + titles[1] + "」之间的关联、差异与责任链"
}

func extractFormattedHeadlineTitles(headlines string, maxItems int) []string {
	if maxItems <= 0 {
		return nil
	}
	matches := formattedHeadlineTitleRE.FindAllStringSubmatch(headlines, -1)
	titles := make([]string, 0, min(maxItems, len(matches)))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		title := cleanHeadlineTitle(match[1])
		if title == "" {
			continue
		}
		titles = append(titles, title)
		if len(titles) >= maxItems {
			break
		}
	}
	return titles
}

func cleanHeadlineTitle(title string) string {
	title = strings.TrimSpace(strings.Join(strings.Fields(title), " "))
	title = strings.Trim(title, "「」“”\"'")
	runes := []rune(title)
	if len(runes) > 42 {
		title = string(runes[:42]) + "…"
	}
	return title
}

func randomInterviewGuest() InterviewGuest {
	return randomInterviewGuestWithRand(rand.Intn)
}

func randomInterviewGuestWithRand(randIntn func(int) int) InterviewGuest {
	if len(InterviewGuests) == 0 {
		return InterviewGuest{}
	}
	return InterviewGuests[randIntn(len(InterviewGuests))]
}

func allTopics() []string {
	var topics []string
	for _, pool := range TopicPools {
		topics = append(topics, pool...)
	}
	return topics
}
