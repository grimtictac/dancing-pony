// Package authz decides who may do what. Every permission check in the
// API goes through Can, so the rules live in one table rather than being
// scattered through handlers.
package authz

// Role is a user's role at one restaurant.
type Role string

const (
	// Customer is any logged-in user with no membership at the restaurant.
	Customer Role = "customer"
	Staff    Role = "staff"
	Owner    Role = "owner"
)

// Action is something a user may try to do at a restaurant.
type Action string

const (
	ViewMenu     Action = "view the menu"
	ManageDishes Action = "manage dishes"
	WriteReview  Action = "write reviews"
)

// Policy is the role × action matrix.
var Policy = map[Action]map[Role]bool{
	ViewMenu:     {Customer: true, Staff: true, Owner: true},
	ManageDishes: {Customer: false, Staff: true, Owner: true},
	// Staff and owners can't review their own restaurant: no self-promotion.
	WriteReview: {Customer: true, Staff: false, Owner: false},
}

// Can reports whether a user with role at a restaurant may perform action.
// Platform admins may also manage any restaurant's dishes (support), but
// get no other extra powers here.
func Can(role Role, platformAdmin bool, action Action) bool {
	if platformAdmin && action == ManageDishes {
		return true
	}
	return Policy[action][role]
}
