package sqlite

import (
	"sort"
	"strings"

	"github.com/levifig/loaf/vnext/continuity"
)

type scratchpadClaimFoldV1 struct {
	claimID         continuity.SubjectID
	participantID   continuity.SubjectID
	resource        string
	expiresAtMillis int64
	root            storedFactV1
	head            storedFactV1
	released        bool
}

func foldScratchpadsV1(corpus projectCorpusV1, atMillis int64) ([]continuity.Scratchpad, error) {
	scratchpads := make([]continuity.Scratchpad, 0)
	for _, subject := range subjectsOfKindV1(corpus, continuity.RecordScratchpad) {
		if err := corpus.contextErrV1(); err != nil {
			return nil, err
		}
		scratchpad, err := foldScratchpadV1(corpus, subject, atMillis)
		if err != nil {
			return nil, err
		}
		scratchpads = append(scratchpads, scratchpad)
	}
	sort.Slice(scratchpads, func(left, right int) bool {
		return factStampLessV1(scratchpads[right].Record.Root, scratchpads[left].Record.Root)
	})
	return scratchpads, nil
}

func foldScratchpadV1(corpus projectCorpusV1, subject continuity.SubjectRef, atMillis int64) (continuity.Scratchpad, error) {
	root, facts, err := canonicalSubjectFactsV1(corpus, subject)
	if err != nil {
		return continuity.Scratchpad{}, err
	}
	opened, err := decodeStoredWireV1[wireScratchpadOpenedV1](string(root.content))
	if err != nil {
		return continuity.Scratchpad{}, corruptFactProblemV1()
	}
	focus := opened.Focus.domainOptional()
	if focus != nil {
		if err := requireEarlierSubjectV1(corpus, root, *focus); err != nil {
			return continuity.Scratchpad{}, err
		}
	}

	participants := make(map[continuity.SubjectID]continuity.ScratchpadParticipant)
	messages := make([]continuity.ScratchpadMessage, 0)
	claims := make(map[continuity.SubjectID]scratchpadClaimFoldV1)
	head := root
	var closeFact storedFactV1
	closeFound := false
	var closePayload wireScratchpadCloseV1

	for _, fact := range facts {
		if err := corpus.contextErrV1(); err != nil {
			return continuity.Scratchpad{}, err
		}
		if fact.kind == continuity.FactScratchpadOpened {
			continue
		}
		if !storedFactLessV1(root, fact) {
			return continuity.Scratchpad{}, corruptFactProblemV1()
		}
		switch fact.kind {
		case continuity.FactScratchpadParticipantIntroduced:
			payload, err := decodeStoredWireV1[wireScratchpadParticipantV1](string(fact.content))
			if err != nil {
				return continuity.Scratchpad{}, corruptFactProblemV1()
			}
			participantID := continuity.SubjectID(payload.ParticipantID)
			if _, exists := participants[participantID]; !exists {
				participantFocus := payload.Focus.domainOptional()
				if participantFocus != nil {
					if err := requireEarlierSubjectV1(corpus, fact, *participantFocus); err != nil {
						return continuity.Scratchpad{}, err
					}
				}
				participants[participantID] = continuity.ScratchpadParticipant{
					Stamp:         factStampV1(fact),
					ParticipantID: participantID,
					Name:          payload.Name,
					Focus:         copyOptionalSubjectV1(participantFocus),
					Observation:   payload.Observation.domain(),
				}
				head = fact
			}
		case continuity.FactScratchpadMessageRecorded:
			payload, err := decodeStoredWireV1[wireScratchpadMessageV1](string(fact.content))
			if err != nil {
				return continuity.Scratchpad{}, corruptFactProblemV1()
			}
			participantID := continuity.SubjectID(payload.ParticipantID)
			if _, exists := participants[participantID]; !exists {
				return continuity.Scratchpad{}, corruptFactProblemV1()
			}
			messages = append(messages, continuity.ScratchpadMessage{
				Stamp:         factStampV1(fact),
				ParticipantID: participantID,
				Text:          payload.Text,
				Observation:   payload.Observation.domain(),
			})
			head = fact
		case continuity.FactScratchpadClaimRecorded:
			payload, err := decodeStoredWireV1[wireScratchpadClaimV1](string(fact.content))
			if err != nil {
				return continuity.Scratchpad{}, corruptFactProblemV1()
			}
			participantID := continuity.SubjectID(payload.ParticipantID)
			if _, exists := participants[participantID]; !exists {
				return continuity.Scratchpad{}, corruptFactProblemV1()
			}
			claimID := continuity.SubjectID(payload.ClaimID)
			claim, exists := claims[claimID]
			if !exists {
				claims[claimID] = scratchpadClaimFoldV1{
					claimID:         claimID,
					participantID:   participantID,
					resource:        payload.Resource,
					expiresAtMillis: payload.ExpiresAtMillis,
					root:            fact,
					head:            fact,
				}
				head = fact
				continue
			}
			if claim.participantID != participantID || claim.resource != payload.Resource {
				continue
			}
			if claim.released {
				continue
			}
			if payload.ExpiresAtMillis >= claim.expiresAtMillis {
				claim.expiresAtMillis = payload.ExpiresAtMillis
				claim.head = fact
				claims[claimID] = claim
				head = fact
			}
		case continuity.FactScratchpadClaimReleased:
			payload, err := decodeStoredWireV1[wireScratchpadClaimReleaseV1](string(fact.content))
			if err != nil {
				return continuity.Scratchpad{}, corruptFactProblemV1()
			}
			claimID := continuity.SubjectID(payload.ClaimID)
			claim, exists := claims[claimID]
			if !exists {
				return continuity.Scratchpad{}, corruptFactProblemV1()
			}
			if claim.participantID != continuity.SubjectID(payload.ReleasedBy) {
				continue
			}
			claim.released = true
			claims[claimID] = claim
			head = fact
		case continuity.FactScratchpadClosed:
			payload, err := decodeStoredWireV1[wireScratchpadCloseV1](string(fact.content))
			if err != nil {
				return continuity.Scratchpad{}, corruptFactProblemV1()
			}
			closeFact = fact
			closePayload = payload
			closeFound = true
		default:
			return continuity.Scratchpad{}, corruptFactProblemV1()
		}
	}

	view := continuity.Scratchpad{
		Focus:             copyOptionalSubjectV1(focus),
		Label:             opened.Label,
		State:             continuity.ScratchpadOpen,
		OpenedObservation: opened.Observation.domain(),
		Participants:      make([]continuity.ScratchpadParticipant, 0, len(participants)),
		Messages:          append([]continuity.ScratchpadMessage(nil), messages...),
		Claims:            make([]continuity.ScratchpadClaim, 0, len(claims)),
	}
	for _, participant := range participants {
		view.Participants = append(view.Participants, participant)
	}
	sort.Slice(view.Participants, func(left, right int) bool {
		return strings.Compare(string(view.Participants[left].ParticipantID), string(view.Participants[right].ParticipantID)) < 0
	})
	for _, claim := range claims {
		if claim.released || claim.expiresAtMillis <= atMillis {
			continue
		}
		observation, err := observationForStoredFactV1(claim.head)
		if err != nil {
			return continuity.Scratchpad{}, corruptFactProblemV1()
		}
		view.Claims = append(view.Claims, continuity.ScratchpadClaim{
			ClaimID:         claim.claimID,
			ParticipantID:   claim.participantID,
			Resource:        claim.resource,
			ExpiresAtMillis: claim.expiresAtMillis,
			Root:            factStampV1(claim.root),
			Head:            factStampV1(claim.head),
			HeadObservation: observation,
		})
	}
	sort.Slice(view.Claims, func(left, right int) bool {
		if view.Claims[left].Resource != view.Claims[right].Resource {
			return strings.Compare(view.Claims[left].Resource, view.Claims[right].Resource) < 0
		}
		return strings.Compare(string(view.Claims[left].ClaimID), string(view.Claims[right].ClaimID)) < 0
	})
	if closeFound {
		head = closeFact
		view.State = continuity.ScratchpadClosed
		view.ClosedBy = continuity.SubjectID(closePayload.ClosedBy)
		view.CloseReason = closePayload.Reason
		view.Participants = []continuity.ScratchpadParticipant{}
		view.Messages = []continuity.ScratchpadMessage{}
		view.Claims = []continuity.ScratchpadClaim{}
	}
	headObservation, err := observationForStoredFactV1(head)
	if err != nil {
		return continuity.Scratchpad{}, corruptFactProblemV1()
	}
	view.Record = recordVersionV1(root, head)
	view.HeadObservation = headObservation
	return view, nil
}
