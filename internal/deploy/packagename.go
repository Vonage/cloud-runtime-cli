package deploy

import "strings"

// packageName is <project>-<instance> as a Docker image name component: lowercase letters and digits, with each
// run of anything else turned into one "-" and none at either end. It is empty when neither name has a letter or
// digit.
func packageName(project, instance string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(project + "-" + instance) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
			continue
		}
		dash = true
	}
	return b.String()
}
