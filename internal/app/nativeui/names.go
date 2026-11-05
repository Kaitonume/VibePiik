package nativeui

// labelNames is labelParticipantNames: people with the same display name get
// the shortest distinct end of their peer ID (at least 6 characters).
func labelNames(ids, names []string) []string {
	peersByName := map[string]map[string]bool{}
	for i, name := range names {
		if peersByName[name] == nil {
			peersByName[name] = map[string]bool{}
		}
		peersByName[name][ids[i]] = true
	}
	lengths := make([]int, len(ids))
	for i, id := range ids {
		lengths[i] = min(6, len(id))
	}
	duplicate := func(i int) bool { return len(peersByName[names[i]]) > 1 }
	for {
		collisions := map[string][]int{}
		for i := range ids {
			if duplicate(i) {
				key := names[i] + "\x00" + ids[i][len(ids[i])-lengths[i]:]
				collisions[key] = append(collisions[key], i)
			}
		}
		extended := false
		for _, indexes := range collisions {
			distinct := map[string]bool{}
			for _, i := range indexes {
				distinct[ids[i]] = true
			}
			if len(distinct) < 2 {
				continue
			}
			for _, i := range indexes {
				if lengths[i] < len(ids[i]) {
					lengths[i]++
					extended = true
				}
			}
		}
		if !extended {
			break
		}
	}
	labels := make([]string, len(ids))
	for i := range ids {
		labels[i] = names[i]
		if duplicate(i) {
			labels[i] = names[i] + " (" + ids[i][len(ids[i])-lengths[i]:] + ")"
		}
	}
	return labels
}
