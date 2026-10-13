package injury

type ChangeDirection string

const (
	DirectionImproved ChangeDirection = "improved"
	DirectionWorsened ChangeDirection = "worsened"
	DirectionNeutral  ChangeDirection = "neutral"
)

type PracticeChange struct {
	Player PlayerReport
	From   PracticeStatus
	To     PracticeStatus
	Trend  ChangeDirection
}

type InjuryChange struct {
	Player PlayerReport
	From   string
	To     string
}

type GameStatusChange struct {
	Player PlayerReport
	From   GameStatus
	To     GameStatus
}

type ReportChanges struct {
	Initial           bool
	Final             bool
	Added             []PlayerReport
	Removed           []PlayerReport
	PracticeChanges   []PracticeChange
	InjuryChanges     []InjuryChange
	GameStatusChanges []GameStatusChange
	// Opponent holds the changes of the opponent section, when present.
	Opponent *ReportChanges
}

func (c ReportChanges) Empty() bool {
	return c.teamEmpty() && (c.Opponent == nil || c.Opponent.teamEmpty())
}

func (c ReportChanges) teamEmpty() bool {
	return !c.Initial &&
		len(c.Added) == 0 &&
		len(c.Removed) == 0 &&
		len(c.PracticeChanges) == 0 &&
		len(c.InjuryChanges) == 0 &&
		len(c.GameStatusChanges) == 0
}

type ReportDiffer struct{}

func (d ReportDiffer) Diff(previous *Report, current Report) ReportChanges {
	changes := d.diffTeam(previous, current)
	if current.OpponentName != "" {
		// A previous snapshot without this opponent counts as its first report.
		var prevOpp *Report
		if previous != nil && previous.OpponentName == current.OpponentName {
			r := previous.OpponentReport()
			prevOpp = &r
		}
		opp := d.diffTeam(prevOpp, current.OpponentReport())
		changes.Opponent = &opp
	}
	return changes
}

func (ReportDiffer) diffTeam(previous *Report, current Report) ReportChanges {
	if previous == nil {
		return ReportChanges{Initial: true, Final: hasFinalDesignations(current), Added: current.Players}
	}
	changes := ReportChanges{Final: hasFinalDesignations(current)}
	prevPlayers := map[string]PlayerReport{}
	for _, p := range previous.Players {
		prevPlayers[p.stableID()] = p
	}
	currentPlayers := map[string]PlayerReport{}
	for _, p := range current.Players {
		currentPlayers[p.stableID()] = p
		old, found := prevPlayers[p.stableID()]
		if !found {
			changes.Added = append(changes.Added, p)
			continue
		}
		if old.LatestPractice() != p.LatestPractice() {
			changes.PracticeChanges = append(changes.PracticeChanges, PracticeChange{
				Player: p,
				From:   old.LatestPractice(),
				To:     p.LatestPractice(),
				Trend:  practiceDirection(old.LatestPractice(), p.LatestPractice()),
			})
		}
		// Sources word injuries differently ("Right Hamstring" vs "Hamstring"),
		// so a source switch alone must not publish an update.
		if old.Injury != p.Injury && previous.Source == current.Source {
			changes.InjuryChanges = append(changes.InjuryChanges, InjuryChange{Player: p, From: old.Injury, To: p.Injury})
		}
		if old.GameStatus != p.GameStatus {
			changes.GameStatusChanges = append(changes.GameStatusChanges, GameStatusChange{Player: p, From: old.GameStatus, To: p.GameStatus})
		}
	}
	for _, p := range previous.Players {
		if _, ok := currentPlayers[p.stableID()]; !ok {
			changes.Removed = append(changes.Removed, p)
		}
	}
	return changes
}

func practiceDirection(from, to PracticeStatus) ChangeDirection {
	rank := map[PracticeStatus]int{PracticeDNP: 0, PracticeLP: 1, PracticeFP: 2}
	fromRank, okFrom := rank[from]
	toRank, okTo := rank[to]
	if !okFrom || !okTo || fromRank == toRank {
		return DirectionNeutral
	}
	if toRank > fromRank {
		return DirectionImproved
	}
	return DirectionWorsened
}

func hasFinalDesignations(report Report) bool {
	for _, p := range report.Players {
		if p.GameStatus == GameStatusOut || p.GameStatus == GameStatusDoubtful || p.GameStatus == GameStatusQuestionable {
			return true
		}
	}
	return false
}
