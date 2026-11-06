package terminalguard

// BacklogReviewLink stands in for the real link: its one defining file is this one.
type BacklogReviewLink struct{ session string }

// newBacklogReviewLink is the sanctioned constructor, in the defining file.
func newBacklogReviewLink() BacklogReviewLink { return BacklogReviewLink{session: "x"} }
