package nativeui

import (
	"regexp"
	"strings"
)

// PIIK TV texts (src/client/lib/piiktv-copy.ts), in the language Piik shows:
// Chinese with 中, English otherwise.
var piikTvCopy = map[string]map[string]string{
	"en": {
		"send.label": "Send URL to PIIK TV app", "send.sentLabel": "Sent to TV",
		"send.hint": "Send the room link to the PIIK TV app", "send.noRoom": "Create a room first",
		"send.sentTo": "Sent to {names}", "send.noTv": "No TV with the PIIK TV app open on this network",
		"send.failed": "Sending failed ({detail})", "send.appDown": "Sending failed: the Piik app on this PC is not responding",
		"card.title": "Watch on TV with the PIIK TV app", "card.placeholder": "Room link, also someone else's",
		"card.inputLabel": "Room link to watch on the TV", "card.button": "Send to TV", "card.searching": "Looking for the TV…",
		"card.invalidExample": "Invalid link: it must be a Piik room link, for example https://…/r/1234#v=…",
		"card.invalid":        "Invalid link: it must be a Piik room link",
		"card.hint":           "Paste the invite link of any Piik room: the TV with the PIIK TV app open opens it right away, even without starting to share on this PC.",
	},
	"zh": {
		"send.label": "将链接发送到 PIIK TV 应用", "send.sentLabel": "已发送到电视",
		"send.hint": "将房间链接发送到 PIIK TV 应用", "send.noRoom": "请先创建房间",
		"send.sentTo": "已发送到 {names}", "send.noTv": "此网络中没有打开 PIIK TV 应用的电视",
		"send.failed": "发送失败（{detail}）", "send.appDown": "发送失败：此电脑上的 Piik 应用没有响应",
		"card.title": "使用 PIIK TV 应用在电视上观看", "card.placeholder": "房间链接，也可以是别人的",
		"card.inputLabel": "要在电视上观看的房间链接", "card.button": "发送到电视", "card.searching": "正在寻找电视…",
		"card.invalidExample": "链接无效：必须是 Piik 房间链接，例如 https://…/r/1234#v=…",
		"card.invalid":        "链接无效：必须是 Piik 房间链接",
		"card.hint":           "粘贴任意 Piik 房间的邀请链接：打开 PIIK TV 应用的电视会立即打开它，无需在此电脑上开始共享。",
	},
}

func piikTvText(uiLanguage, key string, vars ...string) string {
	language := "en"
	switch uiLanguage {
	case "zh", "it":
		language = uiLanguage
	}
	value := piikTvCopy[language][key]
	for i := 0; i+1 < len(vars); i += 2 {
		value = strings.ReplaceAll(value, "{"+vars[i]+"}", vars[i+1])
	}
	return value
}

func init() { piikTvCopy["it"] = piikTvCopy_it }

var roomLink = regexp.MustCompile(`^https?://[A-Za-z0-9.-]+(?::\d{1,5})?/r/[1-9][0-9]{3}[^\s]*$`)
var hasScheme = regexp.MustCompile(`(?i)^[a-z][a-z0-9+.-]*://`)

// piikTvLink is PiikTvLinkCard's piikTvLink: https:// is added when missing.
func piikTvLink(value string) (string, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return "", false
	}
	if !hasScheme.MatchString(text) {
		text = "https://" + text
	}
	return text, len(text) <= 1024 && roomLink.MatchString(text)
}
