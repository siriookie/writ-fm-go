package generator

import "math/rand"

// InterviewGuest is a fictional/composite guest profile for interview segments.
type InterviewGuest struct {
	Name    string
	Context string
}

// TopicPools organizes candidate topics by show topic focus.
var TopicPools = map[string][]string{
	"philosophy": {
		"The 3am mind - why we think differently in darkness",
		"Alone together - the paradox of mass media intimacy",
		"The archaeology of memory - how songs excavate the past",
		"Waiting rooms of the soul - the liminal spaces we inhabit",
		"The democracy of insomnia - who else is awake right now",
	},
	"music_history": {
		"The secret history of the B-side - when the throwaway becomes the classic",
		"How geography shaped sound - the cities that invented genres",
		"Recording studios as instruments - rooms that shaped decades of music",
		"Pirate radio - outlaws of the airwaves and the sounds they set free",
		"The DJ as curator - the art of selection and sequence",
	},
	"current_events": {
		"What the headlines aren't telling you this week",
		"The economy of attention - who benefits when we're distracted",
		"Technology and trust - the crisis nobody's naming",
		"Climate reports and the language of urgency",
		"The state of journalism at the end of the world",
	},
	"culture": {
		"The coffee shop as third place - where strangers become regulars",
		"Night shift workers - the invisible economy that keeps everything running",
		"Diners at 2am - confessionals with unlimited refills",
		"Bookstores as sanctuaries - the quiet resistance of print",
		"The art of the mix tape - playlists as unsent letters",
	},
	"soul_music": {
		"What makes a song 'soul' - it's not a genre, it's an approach",
		"The gospel roots that feed every groove",
		"Funk as philosophy - Parliament and the mothership connection",
		"Erykah Badu and the church of vibe",
		"Disco's death and resurrection - who killed the dance floor and who brought it back",
	},
	"night_philosophy": {
		"What the dark knows that the light doesn't",
		"Dreams as the radio station of the subconscious",
		"The 4am confession - why truth comes easier in darkness",
		"Insomnia as unwanted clarity",
		"Why creativity peaks after midnight",
	},
	"listeners": {
		"Letters from the frequency - your messages answered",
		"The songs that changed your lives - listener stories",
		"Questions from the dark - what you've always wanted to know",
		"Dedications and confessions from the inbox",
		"Where are you listening from? - the geography of our audience",
	},
}

// InterviewGuests mirrors the Python generator's fictional guest pool.
var InterviewGuests = []InterviewGuest{
	{Name: "a retired record store owner from Detroit", Context: "Spent 40 years curating vinyl for a neighborhood"},
	{Name: "a sound engineer who worked on legendary sessions", Context: "Was in the room when history was made on tape"},
	{Name: "a radio historian", Context: "Studies the golden age of pirate and community radio"},
	{Name: "a jazz archivist from a university collection", Context: "Cataloging a century of forgotten recordings"},
	{Name: "a night shift nurse who listens to us every night", Context: "Knows the hospital's secret soundtrack"},
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
