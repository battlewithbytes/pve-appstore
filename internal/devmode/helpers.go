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
