package devmode

import "strings"

// toKebabCase converts a string to kebab-case (lowercase, hyphens).
func toKebabCase(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, s)
	// Collapse multiple dashes
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

// toSnakeCase converts a string to snake_case (lowercase, underscores).
func toSnakeCase(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, s)
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}
	return strings.Trim(s, "_")
}

// stripMarkdownLinks removes markdown-style links like "Name(url)" → "Name".
// Unraid templates often have inline links like "Qbittorrent(https://...)".
func stripMarkdownLinks(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		// Look for pattern: word(http...)
		if s[i] == '(' && i > 0 && s[i-1] != ' ' {
			// Check if content looks like a URL
			end := strings.Index(s[i:], ")")
			if end > 0 {
				inner := s[i+1 : i+end]
				if strings.HasPrefix(inner, "http://") || strings.HasPrefix(inner, "https://") {
					i = i + end + 1
					continue
				}
			}
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

// StripHTML removes HTML tags from a string.
func StripHTML(s string) string {
	var out strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			out.WriteRune(r)
		}
	}
	return strings.TrimSpace(out.String())
}

// envKeyToLabel converts an ENV var key like "OLLAMA_HOST" to a human-readable
// label like "Ollama Host".
func envKeyToLabel(key string) string {
	parts := strings.Split(strings.ToLower(key), "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// mapUnraidCategory maps an Unraid category string to app store categories.
// Unraid uses "Network:Other Media:Other" etc. We extract the primary prefix.
func mapUnraidCategory(cat string) []string {
	mapping := map[string]string{
		"network":      "networking",
		"mediaapp":     "media",
		"mediavideo":   "media",
		"mediaaudio":   "media",
		"mediaserver":  "media",
		"media":        "media",
		"productivity": "productivity",
		"tools":        "tools",
		"utilities":    "utilities",
		"security":     "security",
		"backup":       "backup",
		"cloud":        "cloud",
		"gameservers":  "gaming",
		"homeauto":     "automation",
		"voip":         "communication",
		"webservers":   "web",
		"dns":          "networking",
		"status":       "monitoring",
	}

	seen := map[string]bool{}
	var result []string

	// Unraid categories are space-separated, e.g. "Network:Other Other:"
	for _, part := range strings.Fields(cat) {
		key := strings.ToLower(strings.TrimRight(strings.Split(part, ":")[0], " "))
		if mapped, ok := mapping[key]; ok && !seen[mapped] {
			seen[mapped] = true
			result = append(result, mapped)
		}
	}
	if len(result) == 0 {
		return []string{"utilities"}
	}
	return result
}

// parseWebUIPath extracts the path suffix from an Unraid WebUI template.
// e.g. "http://[IP]:[PORT:8155]/admin" → "/admin"
func parseWebUIPath(webUI string) string {
	if webUI == "" {
		return ""
	}
	// Find the last ] which closes [PORT:NNNN]
	idx := strings.LastIndex(webUI, "]")
	if idx < 0 {
		return ""
	}
	path := webUI[idx+1:]
	if path == "" || path == "/" {
		return ""
	}
	return path
}

// parseCapAdds extracts --cap-add values from an ExtraParams string.
// e.g. "--cap-add NET_ADMIN --cap-add SYS_ADMIN" → ["NET_ADMIN", "SYS_ADMIN"]
func parseCapAdds(extraParams string) []string {
	var caps []string
	parts := strings.Fields(extraParams)
	for i := 0; i < len(parts)-1; i++ {
		if strings.ToLower(parts[i]) == "--cap-add" {
			caps = append(caps, parts[i+1])
		}
	}
	return caps
}

// extractURLFromRepoLine extracts the base URL from a deb repo line.
func extractURLFromRepoLine(line string) string {
	for _, tok := range strings.Fields(line) {
		if strings.HasPrefix(tok, "http://") || strings.HasPrefix(tok, "https://") {
			// Return up to the domain + first path segment
			u := tok
			if idx := strings.Index(u[8:], "/"); idx > 0 {
				return u[:8+idx+1]
			}
			return u + "/"
		}
	}
	return ""
}
