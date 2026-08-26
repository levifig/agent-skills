package scratchpad

import "github.com/levifig/loaf/internal/state"

const (
	KindIntro   = "scratchpad_intro"
	KindMessage = "scratchpad_message"
	KindClaim   = "scratchpad_claim"
	KindRelease = "scratchpad_release"
	KindClose   = "scratchpad_close"
)

func init() {
	state.RegisterFactKind(KindIntro, state.PermanenceScratchpad)
	state.RegisterFactKind(KindMessage, state.PermanenceScratchpad)
	state.RegisterFactKind(KindClaim, state.PermanenceScratchpad)
	state.RegisterFactKind(KindRelease, state.PermanenceScratchpad)
	state.RegisterFactKind(KindClose, state.PermanenceScratchpad)
}
