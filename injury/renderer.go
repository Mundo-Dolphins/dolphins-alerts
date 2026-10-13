package injury

import (
	"fmt"
	"strings"

	"github.com/Mundo-Dolphins/dolphins-alerts/espn"
)

type TelegramRenderer struct{}

func (TelegramRenderer) Render(report Report, changes ReportChanges) string {
	if report.OpponentName != "" && changes.Opponent != nil {
		return renderCombinedTelegram(report, changes)
	}
	if changes.Initial {
		return renderInitialTelegram(report)
	}
	if changes.Final {
		return renderFinalTelegram(report, changes)
	}
	return renderUpdateTelegram(changes)
}

func renderInitialTelegram(report Report) string {
	groups := groupByPractice(report.Players)
	var b strings.Builder
	fmt.Fprintf(&b, "🏥 Primer Injury Report de la semana para los Dolphins")
	if report.Opponent != "" {
		fmt.Fprintf(&b, " antes del partido contra %s", report.Opponent)
	}
	b.WriteString(".")
	writePracticeGroup(&b, "🔴 No entrenaron:", groups[PracticeDNP])
	writePracticeGroup(&b, "🟡 Limitados:", groups[PracticeLP])
	writePracticeGroup(&b, "🟢 Participación completa:", groups[PracticeFP])
	appendSource(&b, report)
	return strings.TrimSpace(b.String())
}

func renderFinalTelegram(report Report, _ ReportChanges) string {
	groups := groupByGameStatus(report.Players)
	if len(groups[GameStatusOut])+len(groups[GameStatusDoubtful])+len(groups[GameStatusQuestionable]) == 0 {
		return "🏥 Miami llega al partido sin jugadores con designación final de lesión."
	}
	var b strings.Builder
	b.WriteString("🏥 Injury Report final de Miami")
	if report.Opponent != "" {
		fmt.Fprintf(&b, " para el partido ante %s", report.Opponent)
	}
	b.WriteString(":\n")
	writeGameGroup(&b, "❌ OUT", groups[GameStatusOut])
	writeGameGroup(&b, "⚠️ DOUBTFUL", groups[GameStatusDoubtful])
	writeGameGroup(&b, "❓ QUESTIONABLE", groups[GameStatusQuestionable])
	appendSource(&b, report)
	return strings.TrimSpace(b.String())
}

func renderUpdateTelegram(changes ReportChanges) string {
	var b strings.Builder
	b.WriteString("🏥 Cambios en el Injury Report de Miami:\n")
	for _, line := range changeLines(changes) {
		b.WriteString("\n" + line)
	}
	return strings.TrimSpace(b.String())
}

func changeLines(changes ReportChanges) []string {
	var lines []string
	for _, p := range changes.Added {
		lines = append(lines, fmt.Sprintf("➕ %s entra en el reporte: %s, %s.", p.Name, TranslateInjury(p.Injury), practiceSpanish(p.LatestPractice())))
	}
	for _, p := range changes.Removed {
		lines = append(lines, fmt.Sprintf("➖ %s ya no aparece en el reporte.", p.Name))
	}
	for _, c := range changes.PracticeChanges {
		icon := "⚪"
		if c.Trend == DirectionImproved {
			icon = "🟢"
		} else if c.Trend == DirectionWorsened {
			icon = "🔴"
		}
		lines = append(lines, fmt.Sprintf("%s %s: %s → %s.", icon, c.Player.Name, practiceSpanish(c.From), practiceSpanish(c.To)))
	}
	for _, c := range changes.InjuryChanges {
		lines = append(lines, fmt.Sprintf("🔄 %s cambia su lesión registrada: %s → %s.", c.Player.Name, TranslateInjury(c.From), TranslateInjury(c.To)))
	}
	for _, c := range changes.GameStatusChanges {
		lines = append(lines, fmt.Sprintf("⚕️ %s cambia su designación: %s → %s.", c.Player.Name, c.From, c.To))
	}
	return lines
}

// renderCombinedTelegram renders Miami and the next opponent in one message.
// Every publication shows both teams' current report.
func renderCombinedTelegram(report Report, changes ReportChanges) string {
	opponent := report.OpponentReport()
	oppChanges := *changes.Opponent
	miamiHeader := "🐬 MIAMI"
	oppHeader := espn.TeamEmoji(report.OpponentName) + " " + strings.ToUpper(report.OpponentName)
	matchup := "Dolphins vs " + report.OpponentName

	var b strings.Builder
	switch {
	case changes.Final || oppChanges.Final:
		b.WriteString("🏥 Injury Report final: " + matchup)
		writeFinalSection(&b, miamiHeader, report.Players)
		writeFinalSection(&b, oppHeader, opponent.Players)
	case changes.Initial:
		b.WriteString("🏥 Primer Injury Report de la semana: " + matchup)
		writePracticeSection(&b, miamiHeader, report.Players)
		writePracticeSection(&b, oppHeader, opponent.Players)
	default:
		b.WriteString("🏥 Injury Report actualizado: " + matchup)
		writePracticeSection(&b, miamiHeader, report.Players)
		writePracticeSection(&b, oppHeader, opponent.Players)
	}
	appendSource(&b, report)
	return strings.TrimSpace(b.String())
}

func writePracticeSection(b *strings.Builder, header string, players []PlayerReport) {
	groups := groupByPractice(players)
	writeSection(b, header, []sectionGroup{
		{"🔴 No entrenaron:", groups[PracticeDNP]},
		{"🟡 Limitados:", groups[PracticeLP]},
		{"🟢 Participación completa:", groups[PracticeFP]},
	}, "Sin jugadores en el reporte.")
}

func writeFinalSection(b *strings.Builder, header string, players []PlayerReport) {
	groups := groupByGameStatus(players)
	writeSection(b, header, []sectionGroup{
		{"❌ OUT", groups[GameStatusOut]},
		{"⚠️ DOUBTFUL", groups[GameStatusDoubtful]},
		{"❓ QUESTIONABLE", groups[GameStatusQuestionable]},
	}, "Sin designaciones finales de lesión.")
}

type sectionGroup struct {
	title   string
	players []PlayerReport
}

func writeSection(b *strings.Builder, header string, groups []sectionGroup, empty string) {
	b.WriteString("\n\n" + header)
	written := false
	for _, g := range groups {
		if len(g.players) == 0 {
			continue
		}
		if written {
			b.WriteString("\n")
		}
		b.WriteString("\n" + g.title)
		for _, p := range sortedCopy(g.players) {
			fmt.Fprintf(b, "\n• %s — %s", p.Name, TranslateInjury(p.Injury))
		}
		written = true
	}
	if !written {
		b.WriteString("\n" + empty)
	}
}

func groupByPractice(players []PlayerReport) map[PracticeStatus][]PlayerReport {
	out := map[PracticeStatus][]PlayerReport{}
	for _, p := range players {
		out[p.LatestPractice()] = append(out[p.LatestPractice()], p)
	}
	return out
}

func groupByGameStatus(players []PlayerReport) map[GameStatus][]PlayerReport {
	out := map[GameStatus][]PlayerReport{}
	for _, p := range players {
		out[p.GameStatus] = append(out[p.GameStatus], p)
	}
	return out
}

func writePracticeGroup(b *strings.Builder, title string, players []PlayerReport) {
	if len(players) == 0 {
		return
	}
	b.WriteString("\n\n" + title)
	for _, p := range sortedCopy(players) {
		fmt.Fprintf(b, "\n• %s — %s", p.Name, TranslateInjury(p.Injury))
	}
}

func writeGameGroup(b *strings.Builder, title string, players []PlayerReport) {
	if len(players) == 0 {
		return
	}
	b.WriteString("\n\n" + title)
	for _, p := range sortedCopy(players) {
		fmt.Fprintf(b, "\n• %s — %s", p.Name, TranslateInjury(p.Injury))
	}
}

func appendSource(b *strings.Builder, report Report) {
	if report.Source == SourceNFLVerse {
		b.WriteString("\n\n⚠️ Fuente alternativa (nflverse): la fuente principal no está disponible y estos datos pueden no reflejar el último entrenamiento.")
	}
	if strings.TrimSpace(report.SourceURL) != "" {
		fmt.Fprintf(b, "\n\n🔗 Fuente: %s", report.SourceURL)
	}
}

func sortedCopy(players []PlayerReport) []PlayerReport {
	out := append([]PlayerReport(nil), players...)
	sortPlayers(out)
	return out
}
