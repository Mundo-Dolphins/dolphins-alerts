package injury

import "strings"

func NormalizePracticeStatus(raw string) PracticeStatus {
	s := normalizeToken(raw)
	switch {
	case s == "", s == "-", s == "(-)", s == "N/A", s == "NOT LISTED", s == "NOT SPECIFIED":
		return PracticeNotListed
	case strings.Contains(s, "DID NOT PARTICIPATE"), strings.Contains(s, "DID NOT PRACTICE"), strings.Contains(s, "DNP"), strings.Contains(s, "NO ENTRENO"):
		return PracticeDNP
	case strings.Contains(s, "LIMITED PARTICIPATION"), strings.Contains(s, "LIMITED"), strings.Contains(s, "LP"), strings.Contains(s, "LIMITADO"):
		return PracticeLP
	case strings.Contains(s, "FULL PARTICIPATION"), strings.Contains(s, "FULL"), strings.Contains(s, "FP"), strings.Contains(s, "COMPLETO"):
		return PracticeFP
	default:
		return PracticeUnknown
	}
}

func NormalizeGameStatus(raw string) GameStatus {
	s := normalizeToken(raw)
	switch {
	case s == "", s == "-", s == "N/A", s == "NOT LISTED", s == "NOT SPECIFIED":
		return GameStatusNotListed
	case s == "OUT" || strings.Contains(s, " OUT"):
		return GameStatusOut
	case strings.Contains(s, "DOUBTFUL"):
		return GameStatusDoubtful
	case strings.Contains(s, "QUESTIONABLE"):
		return GameStatusQuestionable
	default:
		return GameStatusUnknown
	}
}

func practiceSpanish(status PracticeStatus) string {
	switch status {
	case PracticeDNP:
		return "no entreno"
	case PracticeLP:
		return "limitado"
	case PracticeFP:
		return "participacion completa"
	default:
		return "no listado"
	}
}

func normalizeToken(s string) string {
	s = strings.TrimSpace(strings.ToUpper(s))
	replacer := strings.NewReplacer("Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U")
	return replacer.Replace(s)
}
