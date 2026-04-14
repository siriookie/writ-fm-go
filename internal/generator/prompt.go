package generator

import (
	"fmt"
	"math/rand"

	"github.com/writ-fm/go/internal/generator/persona"
)

// WordTarget defines the acceptable output length range for a segment type.
type WordTarget struct {
	Min int
	Max int
}

// SegmentWordTargets mirrors the Python generator's target ranges.
var SegmentWordTargets = map[string]WordTarget{
	"deep_dive":        {Min: 1500, Max: 2500},
	"news_analysis":    {Min: 1500, Max: 2000},
	"interview":        {Min: 2000, Max: 3000},
	"panel":            {Min: 2000, Max: 3000},
	"story":            {Min: 1500, Max: 2500},
	"listener_mailbag": {Min: 1500, Max: 2000},
	"music_essay":      {Min: 1500, Max: 2500},
	"station_id":       {Min: 15, Max: 30},
	"show_intro":       {Min: 80, Max: 150},
	"show_outro":       {Min: 60, Max: 120},
}

// SegmentPrompts holds the task-level instruction for each segment type.
var SegmentPrompts = map[string]string{
	"deep_dive": `Write an extended exploration of this topic. Go deep.
Build your central idea through stories, examples, tangents.
Let one thought lead naturally to another. Circle back to earlier threads.
Include specific details: years, names, places when relevant.
Structure: open with a hook, develop through 3-4 connected ideas, land somewhere unexpected.
Use [pause] for natural rhythm. Output ONLY the spoken words.`,
	"news_analysis": `Analyze these headlines through a late-night lens.
Don't just report - interpret. What patterns do you see? What's being missed?
Connect current events to deeper themes. Ask the questions daytime anchors don't.
Be thoughtful, not reactive. Skeptical but not cynical.

HEADLINES:
%s

Use [pause] for natural rhythm. Output ONLY the spoken words.`,
	"interview": `Write a simulated interview where you (the host) talk with %s.
Format with HOST: and GUEST: markers on separate lines.
The guest is a fictional/composite character, not a real living person being impersonated.
The conversation should feel natural - interruptions, tangents, moments of surprise.
Build to genuine insight or revelation.
Use [pause] for natural rhythm. Output ONLY the spoken dialogue.`,
	"panel": `Write a discussion between two hosts on this topic.
Format with HOST_A: and HOST_B: markers on separate lines.
They have different perspectives but mutual respect.
The conversation should build - start with disagreement, find nuance, reach unexpected common ground.
Include moments of genuine surprise and humor.
Use [pause] for natural rhythm. Output ONLY the spoken dialogue.`,
	"story": `Tell a story. It can be true, apocryphal, or mythological - but tell it like it happened.
Good stories have specific details: the color of the room, the year, the weather.
Build tension. Let the listener wonder where this is going.
The ending should reframe everything that came before.
Use [pause] for dramatic effect. Output ONLY the spoken words.`,
	"listener_mailbag": `Write a segment responding to invented listener messages.
Create 2-3 messages from listeners (with first names and cities).
Each message should touch on something real - a memory, a question, a feeling.
Respond to each with genuine warmth and thoughtfulness.
Format: read the message, then respond. Natural transitions between letters.
Use [pause] for natural rhythm. Output ONLY the spoken words.`,
	"music_essay": `Write an extended essay about music.
This is not a review. It's a love letter, an excavation, a meditation.
Pick a specific angle: a single song, a studio, a year, a collaboration, a genre's birth.
Use vivid, sensory language. Make the listener hear what you're describing.
Be specific with details but universal with feeling.
Use [pause] for natural rhythm. Output ONLY the spoken words.`,
	"station_id": `Write a 15-30 word station ID for WRIT-FM.
Be cryptic but warm. Reference the frequency, the signal, the persistence of broadcasting.
Output ONLY the spoken text. No quotes, headers, or explanations.`,
	"show_intro": `Write an 80-150 word opening for the show.
Welcome listeners. Set the mood. Hint at what's ahead without being specific.
Ground the listener in time and space - what hour is it, what kind of night.
Output ONLY the spoken text.`,
	"show_outro": `Write a 60-120 word show closing.
Thank the listener for staying. Acknowledge the time spent together.
Hint at what's next on the station. Leave them with something to carry.
Output ONLY the spoken text.`,
}

// BuildRequest is the input for prompt generation.
type BuildRequest struct {
	HostID          string
	SegmentType     string
	Topic           string
	ShowName        string
	ShowDescription string
	TopicFocus      string
	Headlines       string
	GuestName       string
	GuestContext    string
}

// PromptBuilder assembles generation prompts using injected collaborators.
type PromptBuilder struct {
	buildHostPrompt func(string, *persona.ShowContext) (string, error)
	randIntn        func(int) int
}

// NewPromptBuilder returns a prompt builder with production defaults.
func NewPromptBuilder() *PromptBuilder {
	return NewPromptBuilderWithDeps(nil, nil)
}

// NewPromptBuilderWithDeps returns a prompt builder with injected collaborators.
func NewPromptBuilderWithDeps(
	buildHostPrompt func(string, *persona.ShowContext) (string, error),
	randIntn func(int) int,
) *PromptBuilder {
	if buildHostPrompt == nil {
		buildHostPrompt = persona.BuildHostPrompt
	}
	if randIntn == nil {
		randIntn = rand.Intn
	}

	return &PromptBuilder{
		buildHostPrompt: buildHostPrompt,
		randIntn:        randIntn,
	}
}

// BuildGenerationPrompt assembles the full persona + task prompt.
func BuildGenerationPrompt(req BuildRequest) (string, error) {
	return NewPromptBuilder().Build(req)
}

// Build assembles the full persona + task prompt.
func (b *PromptBuilder) Build(req BuildRequest) (string, error) {
	showCtx := &persona.ShowContext{
		ShowName:        req.ShowName,
		ShowDescription: req.ShowDescription,
		TopicFocus:      req.TopicFocus,
		SegmentType:     req.SegmentType,
	}
	base, err := b.buildHostPrompt(req.HostID, showCtx)
	if err != nil {
		return "", err
	}

	target, ok := SegmentWordTargets[req.SegmentType]
	if !ok {
		target = SegmentWordTargets["deep_dive"]
	}

	segmentPrompt, topic := b.taskPrompt(req)
	return fmt.Sprintf(`%s

SEGMENT: %s
TOPIC: %s
TARGET LENGTH: %d-%d words

%s`, base, req.SegmentType, topic, target.Min, target.Max, segmentPrompt), nil
}

func (b *PromptBuilder) taskPrompt(req BuildRequest) (string, string) {
	switch req.SegmentType {
	case "news_analysis":
		headlines := req.Headlines
		if headlines == "" {
			headlines = "No headlines available - discuss the nature of news itself."
		}
		return fmt.Sprintf(SegmentPrompts["news_analysis"], headlines), req.Topic
	case "interview":
		guestName := req.GuestName
		guestContext := req.GuestContext
		if guestName == "" {
			guest := randomInterviewGuestWithRand(b.randIntn)
			guestName = guest.Name
			guestContext = guest.Context
		}
		topic := req.Topic
		if guestContext != "" {
			topic = fmt.Sprintf("%s (Guest context: %s)", topic, guestContext)
		}
		return fmt.Sprintf(SegmentPrompts["interview"], guestName), topic
	default:
		prompt, ok := SegmentPrompts[req.SegmentType]
		if !ok {
			prompt = SegmentPrompts["deep_dive"]
		}
		return prompt, req.Topic
	}
}
