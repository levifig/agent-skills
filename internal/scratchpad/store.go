package scratchpad

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

type AppendOptions struct {
	Channel    string
	InstanceID string
	EnvID      string
	Who        string
	WorkingRef string
	Text       string
}

type ClaimOptions struct {
	Channel    string
	InstanceID string
	EnvID      string
	Resource   string
	TTL        time.Duration
}

type ReleaseOptions struct {
	Channel    string
	InstanceID string
	EnvID      string
	Resource   string
}

type ChannelView struct {
	Channel     string         `json:"channel"`
	Messages    []Entry        `json:"messages"`
	Roster      []RosterMember `json:"roster"`
	ActiveClaims []ActiveClaim `json:"active_claims"`
}

func AppendMessage(ctx context.Context, root project.Root, resolver state.PathResolver, opts AppendOptions) (state.FactEnvelope, error) {
	channel, err := normalizeChannel(opts.Channel)
	if err != nil {
		return state.FactEnvelope{}, err
	}
	text := strings.TrimSpace(opts.Text)
	if text == "" {
		return state.FactEnvelope{}, fmt.Errorf("scratchpad message cannot be empty")
	}
	instanceID := normalizeInstanceID(opts.InstanceID)
	envID := strings.TrimSpace(opts.EnvID)
	if envID == "" {
		envID = state.LocalFactEnvID()
	}
	store, projectID, err := openProjectStore(ctx, root, resolver)
	if err != nil {
		return state.FactEnvelope{}, err
	}
	defer store.Close()

	introduced, err := instanceIntroduced(ctx, store, projectID, channel, instanceID, envID)
	if err != nil {
		return state.FactEnvelope{}, err
	}
	if !introduced {
		who := strings.TrimSpace(opts.Who)
		if who == "" {
			who = instanceID
		}
		if _, err := appendIntro(ctx, store, projectID, IntroPayload{
			Channel:    channel,
			InstanceID: instanceID,
			EnvID:      envID,
			Who:        who,
			WorkingRef: strings.TrimSpace(opts.WorkingRef),
		}, envID); err != nil {
			return state.FactEnvelope{}, err
		}
	}
	payload, err := encodePayload(MessagePayload{
		Channel:    channel,
		InstanceID: instanceID,
		EnvID:      envID,
		Text:       text,
	})
	if err != nil {
		return state.FactEnvelope{}, err
	}
	return state.AppendFact(ctx, store, state.AppendFactInput{
		ProjectID: projectID,
		Kind:      KindMessage,
		Payload:   payload,
		EnvID:     envID,
	})
}

func Claim(ctx context.Context, root project.Root, resolver state.PathResolver, opts ClaimOptions) (state.FactEnvelope, error) {
	channel, err := normalizeChannel(opts.Channel)
	if err != nil {
		return state.FactEnvelope{}, err
	}
	resource := strings.TrimSpace(opts.Resource)
	if resource == "" {
		return state.FactEnvelope{}, fmt.Errorf("scratchpad claim resource is required")
	}
	if opts.TTL <= 0 {
		return state.FactEnvelope{}, fmt.Errorf("scratchpad claim ttl must be positive")
	}
	instanceID := normalizeInstanceID(opts.InstanceID)
	envID := strings.TrimSpace(opts.EnvID)
	if envID == "" {
		envID = state.LocalFactEnvID()
	}
	store, projectID, err := openProjectStore(ctx, root, resolver)
	if err != nil {
		return state.FactEnvelope{}, err
	}
	defer store.Close()
	expiresAt := time.Now().UTC().Add(opts.TTL).Format(time.RFC3339Nano)
	payload, err := encodePayload(ClaimPayload{
		Channel:    channel,
		InstanceID: instanceID,
		EnvID:      envID,
		Resource:   resource,
		ExpiresAt:  expiresAt,
	})
	if err != nil {
		return state.FactEnvelope{}, err
	}
	return state.AppendFact(ctx, store, state.AppendFactInput{
		ProjectID: projectID,
		Kind:      KindClaim,
		Payload:   payload,
		EnvID:     envID,
	})
}

func Release(ctx context.Context, root project.Root, resolver state.PathResolver, opts ReleaseOptions) (state.FactEnvelope, error) {
	channel, err := normalizeChannel(opts.Channel)
	if err != nil {
		return state.FactEnvelope{}, err
	}
	resource := strings.TrimSpace(opts.Resource)
	if resource == "" {
		return state.FactEnvelope{}, fmt.Errorf("scratchpad release resource is required")
	}
	instanceID := normalizeInstanceID(opts.InstanceID)
	envID := strings.TrimSpace(opts.EnvID)
	if envID == "" {
		envID = state.LocalFactEnvID()
	}
	store, projectID, err := openProjectStore(ctx, root, resolver)
	if err != nil {
		return state.FactEnvelope{}, err
	}
	defer store.Close()
	payload, err := encodePayload(ReleasePayload{
		Channel:    channel,
		InstanceID: instanceID,
		EnvID:      envID,
		Resource:   resource,
	})
	if err != nil {
		return state.FactEnvelope{}, err
	}
	return state.AppendFact(ctx, store, state.AppendFactInput{
		ProjectID: projectID,
		Kind:      KindRelease,
		Payload:   payload,
		EnvID:     envID,
	})
}

func ReadChannel(ctx context.Context, root project.Root, resolver state.PathResolver, channel string, limit int) (ChannelView, error) {
	channel, err := normalizeChannel(channel)
	if err != nil {
		return ChannelView{}, err
	}
	store, projectID, err := openProjectStore(ctx, root, resolver)
	if err != nil {
		return ChannelView{}, err
	}
	defer store.Close()
	facts, err := listChannelFacts(ctx, store, projectID, channel)
	if err != nil {
		return ChannelView{}, err
	}
	view := ChannelView{Channel: channel}
	now := time.Now().UTC()
	claimIndex := map[string]ActiveClaim{}
	releaseIndex := map[string]map[string]bool{}
	rosterIndex := map[string]RosterMember{}

	for _, fact := range facts {
		permanence, ok := state.FactPermanenceClass(fact.Kind)
		if !ok || permanence != state.PermanenceScratchpad {
			continue
		}
		switch fact.Kind {
		case KindIntro:
			payload, err := decodeIntro(fact.Payload)
			if err != nil {
				return ChannelView{}, err
			}
			key := rosterKey(payload.EnvID, payload.InstanceID)
			rosterIndex[key] = RosterMember{
				InstanceID: payload.InstanceID,
				EnvID:      payload.EnvID,
				Who:        payload.Who,
				WorkingRef: payload.WorkingRef,
				LastSeen:   fact.HLC,
			}
		case KindMessage:
			payload, err := decodeMessage(fact.Payload)
			if err != nil {
				return ChannelView{}, err
			}
			key := rosterKey(payload.EnvID, payload.InstanceID)
			if member, ok := rosterIndex[key]; ok {
				member.LastSeen = fact.HLC
				rosterIndex[key] = member
			}
			view.Messages = append(view.Messages, Entry{ID: fact.ID, Kind: fact.Kind, HLC: fact.HLC, Payload: payload})
		case KindClaim:
			payload, err := decodeClaim(fact.Payload)
			if err != nil {
				return ChannelView{}, err
			}
			expiresAt, err := time.Parse(time.RFC3339Nano, payload.ExpiresAt)
			if err != nil {
				return ChannelView{}, err
			}
			if !expiresAt.After(now) {
				continue
			}
			claimIndex[claimKey(payload.Resource, payload.EnvID, payload.InstanceID)] = ActiveClaim{
				Resource:   payload.Resource,
				InstanceID: payload.InstanceID,
				EnvID:      payload.EnvID,
				ExpiresAt:  payload.ExpiresAt,
				ClaimedAt:  fact.HLC,
			}
		case KindRelease:
			payload, err := decodeRelease(fact.Payload)
			if err != nil {
				return ChannelView{}, err
			}
			key := claimKey(payload.Resource, payload.EnvID, payload.InstanceID)
			if releaseIndex[payload.Resource] == nil {
				releaseIndex[payload.Resource] = map[string]bool{}
			}
			releaseIndex[payload.Resource][key] = true
		}
	}

	for key, claim := range claimIndex {
		if releaseIndex[claim.Resource][key] {
			delete(claimIndex, key)
		}
	}
	for _, member := range rosterIndex {
		view.Roster = append(view.Roster, member)
	}
	sort.Slice(view.Roster, func(i, j int) bool {
		return view.Roster[i].InstanceID < view.Roster[j].InstanceID
	})
	for _, claim := range claimIndex {
		view.ActiveClaims = append(view.ActiveClaims, claim)
	}
	sort.Slice(view.ActiveClaims, func(i, j int) bool {
		return view.ActiveClaims[i].Resource < view.ActiveClaims[j].Resource
	})
	if limit > 0 && len(view.Messages) > limit {
		view.Messages = view.Messages[len(view.Messages)-limit:]
	}
	return view, nil
}

func openProjectStore(ctx context.Context, root project.Root, resolver state.PathResolver) (*state.Store, string, error) {
	store, err := state.OpenProjectStoreForWrite(ctx, root, resolver)
	if err != nil {
		return nil, "", err
	}
	projectID, err := store.ProjectIDForRoot(ctx, root)
	if err != nil {
		store.Close()
		return nil, "", err
	}
	return store, projectID, nil
}

func appendIntro(ctx context.Context, store *state.Store, projectID string, payload IntroPayload, envID string) (state.FactEnvelope, error) {
	encoded, err := encodePayload(payload)
	if err != nil {
		return state.FactEnvelope{}, err
	}
	return state.AppendFact(ctx, store, state.AppendFactInput{
		ProjectID: projectID,
		Kind:      KindIntro,
		Payload:   encoded,
		EnvID:     envID,
	})
}

func instanceIntroduced(ctx context.Context, store *state.Store, projectID, channel, instanceID, envID string) (bool, error) {
	facts, err := listChannelFacts(ctx, store, projectID, channel)
	if err != nil {
		return false, err
	}
	for _, fact := range facts {
		if fact.Kind != KindIntro {
			continue
		}
		payload, err := decodeIntro(fact.Payload)
		if err != nil {
			return false, err
		}
		if payload.InstanceID == instanceID && payload.EnvID == envID {
			return true, nil
		}
	}
	return false, nil
}

func listChannelFacts(ctx context.Context, store *state.Store, projectID, channel string) ([]state.FactEnvelope, error) {
	rows, err := store.QueryContext(ctx, `
SELECT id, project_id, kind, payload, env_id, seq, hlc, envelope_v
FROM facts
WHERE project_id = ?
  AND kind IN (?, ?, ?, ?)
ORDER BY hlc ASC, env_id ASC, id ASC
`, projectID, KindIntro, KindMessage, KindClaim, KindRelease)
	if err != nil {
		return nil, fmt.Errorf("list scratchpad facts: %w", err)
	}
	defer rows.Close()
	var facts []state.FactEnvelope
	for rows.Next() {
		var fact state.FactEnvelope
		if err := rows.Scan(&fact.ID, &fact.ProjectID, &fact.Kind, &fact.Payload, &fact.EnvID, &fact.Seq, &fact.HLC, &fact.EnvelopeV); err != nil {
			return nil, fmt.Errorf("scan scratchpad fact: %w", err)
		}
		permanence, _ := state.FactPermanenceClass(fact.Kind)
		fact.Permanence = permanence
		if !factMatchesChannel(fact, channel) {
			continue
		}
		facts = append(facts, fact)
	}
	return facts, rows.Err()
}

func factMatchesChannel(fact state.FactEnvelope, channel string) bool {
	switch fact.Kind {
	case KindIntro:
		payload, err := decodeIntro(fact.Payload)
		return err == nil && payload.Channel == channel
	case KindMessage:
		payload, err := decodeMessage(fact.Payload)
		return err == nil && payload.Channel == channel
	case KindClaim:
		payload, err := decodeClaim(fact.Payload)
		return err == nil && payload.Channel == channel
	case KindRelease:
		payload, err := decodeRelease(fact.Payload)
		return err == nil && payload.Channel == channel
	default:
		return false
	}
}

func rosterKey(envID, instanceID string) string {
	return envID + "\x00" + instanceID
}

func claimKey(resource, envID, instanceID string) string {
	return resource + "\x00" + envID + "\x00" + instanceID
}
