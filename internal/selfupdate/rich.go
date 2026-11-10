package selfupdate

// noticeText is the update notice of a terminal: one line that Notice shows dim on stderr.
func noticeText(current, latest string) string {
	return "Update available " + shownVersion(current) + " → " + latest + " · run vcr upgrade"
}
