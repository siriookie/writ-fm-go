package musicgen

import "math/rand"

// captionEntry is a single caption pool entry for bumper generation.
type captionEntry struct {
	Caption      string
	DisplayName  string
	Instrumental bool
	Lyrics       string // non-empty when Instrumental is false
}

// captionPools maps bumper_style (from schedule.yaml) to a pool of entries.
var captionPools = map[string][]captionEntry{
	"ambient": {
		// Brian Eno 式氛围音乐，缓慢演化的管风琴音色，强调质感与空间感。
		{Caption: "Brian Eno ambient, slow evolving organ tones, texture and space", DisplayName: "Brian Eno", Instrumental: true},
		// 黑暗氛围、持续 drone、金属共振、长混响尾音、偏电影感。
		{Caption: "dark ambient, drone, metallic resonance, long reverb tails, cinematic", DisplayName: "Dark Drone", Instrumental: true},
		// 德式 kosmische 宇宙音乐，缓慢调制的 pad，太空感，无明显节奏。
		{Caption: "kosmische musik, slowly modulating synth pads, outer space, no rhythm", DisplayName: "Kosmische", Instrumental: true},
		// 磁带循环氛围，低保真温暖感，底噪朦胧，偏冥想。
		{Caption: "tape loop ambient, lo-fi warmth, hiss and haze, meditative", DisplayName: "Tape Loop", Instrumental: true},
		// 极简氛围，单个持续音、玻璃质感泛音、留白很多。
		{Caption: "minimal ambient, single sustained note, glass harmonics, silence", DisplayName: "Glass Tone", Instrumental: true},
		// 氛围电子，漂浮琶音，柔和混响，深夜感。
		{Caption: "ambient electronica, floating arpeggios, soft reverb, late night", DisplayName: "Night Float", Instrumental: true},
		// Harold Budd 风格，轻柔钢琴溶进混响，整体稀疏。
		{Caption: "Harold Budd inspired, gentle piano notes dissolving in reverb, sparse", DisplayName: "Harold Budd", Instrumental: true},
		// 后摇氛围，缓慢推进的吉他 swell，无打击乐，地平线般开阔。
		{Caption: "post-rock ambient, slow guitar swells, no percussion, horizon feeling", DisplayName: "Slow Swell", Instrumental: true},
	},
	"jazz": {
		// 深夜爵士，弱音小号、刷镲鼓、低音提琴，烟雾缭绕的小酒馆感。
		{Caption: "late night jazz, muted trumpet, brushed drums, double bass, smoky club", DisplayName: "Late Night Jazz", Instrumental: true},
		// 冷爵士，西海岸风格，钢琴三重奏，慢摇摆，轻松松弛。
		{Caption: "cool jazz, west coast style, piano trio, slow swing, relaxed tempo", DisplayName: "Cool Jazz", Instrumental: true},
		// 巴萨诺瓦爵士，尼龙弦吉他、温柔桑巴节奏、夏日晚风感。
		{Caption: "bossa nova jazz, nylon string guitar, gentle samba rhythm, summer evening", DisplayName: "Bossa Nova", Instrumental: true},
		// 调式爵士，John Coltrane 影响，高音萨克斯，冥想感，开放和弦。
		{Caption: "modal jazz, John Coltrane influence, soprano sax, meditative, open chords", DisplayName: "Modal Jazz", Instrumental: true},
		// Bebop 爵士，快速钢琴、walking bass、紧凑刷镲，偏亲密小编制。
		{Caption: "bebop jazz, fast piano, walking bass, tight brushwork, intimate", DisplayName: "Bebop", Instrumental: true},
		// ECM 厂牌式爵士，北欧极简，稀疏钢琴，长停顿。
		{Caption: "ECM style jazz, Scandinavian minimalism, sparse piano, long silences", DisplayName: "Nordic Jazz", Instrumental: true},
		// 爵士融合，Rhodes 电钢、柔和 groove、电影感。
		{Caption: "jazz fusion, Rhodes electric piano, soft groove, cinematic", DisplayName: "Jazz Fusion", Instrumental: true},
		// 独奏钢琴爵士，Bill Evans 风格，印象派和声，偏内省。
		{Caption: "solo piano jazz, Bill Evans style, impressionistic harmony, reflective", DisplayName: "Solo Piano", Instrumental: true},
	},
	"downtempo": {
		// Trip-hop，Massive Attack 风格，慢速阴暗鼓点、低频和忧郁弦乐。
		{Caption: "trip-hop, Massive Attack style, slow dark beat, bass, melancholic strings", DisplayName: "Trip-Hop", Instrumental: true},
		// 低速电子，70 BPM，温暖 pad，低保真鼓组，沉思感。
		{Caption: "downtempo electronica, 70 BPM, warm pads, lo-fi drums, contemplative", DisplayName: "Downtempo", Instrumental: true},
		// Chill out，Cafe del Mar 风格，木吉他、柔和合成器、落日海边氛围。
		{Caption: "chill out music, Caf茅 del Mar style, acoustic guitar, gentle synth, sunset", DisplayName: "Chill Out", Instrumental: true},
		// Portishead 风格，幽灵般女声采样、黑胶吱呀感、稀疏节拍。
		{Caption: "Portishead influenced, haunting female vocal samples, creaking vinyl, sparse beat", DisplayName: "Haunted Beat", Instrumental: true},
		// Future beats，慢速 808 低频、变调 vocal chops、雾状氛围。
		{Caption: "future beats, slow 808 bass, pitched vocal chops, hazy atmosphere", DisplayName: "Future Beats", Instrumental: true},
		// Bonobo 风格，现场鼓、自然纹理、爵士采样、顺滑律动。
		{Caption: "Bonobo style, live drums, organic textures, jazz samples, smooth groove", DisplayName: "Organic Beats", Instrumental: true},
		// Lo-fi hip hop，磁带温度感、软钢琴、雨声、深夜学习场景。
		{Caption: "lo-fi hip hop, cassette warmth, soft piano, rain sounds, late study session", DisplayName: "Lo-Fi Study", Instrumental: true},
		// Dub techno，Basic Channel 风格，深混响、极简脉冲、催眠感。
		{Caption: "dub techno, Basic Channel style, deep reverb, minimal pulse, hypnotic", DisplayName: "Dub Techno", Instrumental: true},
	},
	"soul": {
		// 经典灵魂乐，Motown 风格，风琴、铜管、复古温暖音色。
		{Caption: "classic soul, Motown style, organ, brass section, warm vintage sound", DisplayName: "Classic Soul", Instrumental: true},
		// Neo soul，D'Angelo 风格，和弦丰厚，律动紧致，偏感性。
		{Caption: "neo soul, D'Angelo influence, lush chords, tight groove, sensual", DisplayName: "Neo Soul", Instrumental: true},
		// 费城灵魂乐，华丽弦乐、稳健节奏、上扬情绪、70 年代质感。
		{Caption: "Philadelphia soul, lush strings, steady rhythm, uplifting, 1970s feel", DisplayName: "Philly Soul", Instrumental: true},
		// Gospel soul，Hammond 风琴、应答式唱法、欢乐、厚合唱铺底。
		{Caption: "gospel soul, Hammond organ, call and response, joyful, full choir pads", DisplayName: "Gospel Soul", Instrumental: true},
		// Funk soul，James Brown 影响，紧铜管、切分吉他、强 groove。
		{Caption: "funk soul, James Brown influence, tight brass, choppy guitar, heavy groove", DisplayName: "Funk Soul", Instrumental: true},
		// 南方灵魂乐，粗粝吉他、Wurlitzer 电钢、情绪外放、夜色热度。
		{Caption: "southern soul, raw guitar, wurlitzer piano, emotional, late night heat", DisplayName: "Southern Soul", Instrumental: true},
		// Quiet storm 式 R&B，顺滑萨克斯、慢速、浪漫氛围。
		{Caption: "quiet storm R&B, smooth saxophone, slow tempo, romantic atmosphere", DisplayName: "Quiet Storm", Instrumental: true},
		// Curtis Mayfield 风格，wah 吉他、弦乐、带社会意识的律动。
		{Caption: "Curtis Mayfield inspired, wah guitar, strings, socially conscious groove", DisplayName: "Curtis Mayfield", Instrumental: true},
	},
	"indie": {
		// Indie rock，清亮拨弦吉他、混响、中速、略带忧郁。
		{Caption: "indie rock, jangly guitars, reverb, mid-tempo, slightly melancholic", DisplayName: "Indie Rock", Instrumental: true},
		// Shoegaze，吉他墙、梦幻人声埋在混响里、空灵。
		{Caption: "shoegaze, wall of guitars, dreamy vocals buried in reverb, ethereal", DisplayName: "Shoegaze", Instrumental: true},
		// Post-punk，棱角吉他、重低音贝斯、稀疏鼓、张力感。
		{Caption: "post-punk, angular guitars, bass-heavy, sparse drums, tension", DisplayName: "Post-Punk", Instrumental: true},
		// Indie folk，木吉他、指弹、亲密感、咖啡馆暖意。
		{Caption: "indie folk, acoustic guitar, fingerpicking, intimate, coffeehouse warmth", DisplayName: "Indie Folk", Instrumental: true},
		// Dream pop，Cocteau Twins 风格，叮咚吉他、丰厚混响、异世界感。
		{Caption: "dream pop, Cocteau Twins style, chiming guitars, lush reverb, otherworldly", DisplayName: "Dream Pop", Instrumental: true},
		// Lo-fi indie，卧室录音质感、磁带底噪、真诚粗粝。
		{Caption: "lo-fi indie, home recording warmth, tape hiss, honest and raw", DisplayName: "Lo-Fi Indie", Instrumental: true},
		// Math rock，复杂拍号、干净交错吉他、偏理性脑性。
		{Caption: "math rock, complex time signatures, clean interlocking guitars, cerebral", DisplayName: "Math Rock", Instrumental: true},
		// Indie electronic，合成器与吉他混合，忧郁旋律，夜路驾驶感。
		{Caption: "indie electronic, synth and guitar blend, melancholy melody, night drive", DisplayName: "Indie Electronic", Instrumental: true},
	},
}

// pickCaption selects a random caption entry for the given bumper style.
// Falls back to "ambient" for unknown styles.
func pickCaption(style string, rng *rand.Rand) captionEntry {
	pool, ok := captionPools[style]
	if !ok {
		pool = captionPools["ambient"]
	}
	return pool[rng.Intn(len(pool))]
}
