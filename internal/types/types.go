package types

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type UsernameHistory struct {
	Username string `json:"username"`
	Date     string `json:"date"`
}

type IDHistory struct {
	ID   int64  `json:"id"`
	Date string `json:"date"`
}

type Meta struct {
	SearchQuery    string `json:"search_query"`
	KnownNumGroups int    `json:"known_num_groups"`
	NumGroups      int    `json:"num_groups"`
	OpCost         int    `json:"op_cost"`
}

type Group struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Title       string `json:"title"`
	DateUpdated string `json:"date_updated"`
}

type TGScanResponse struct {
	Status string `json:"status"`
	Result struct {
		User            User             `json:"user"`
		UsernameHistory []UsernameHistory `json:"username_history"`
		IDHistory       []IDHistory      `json:"id_history"`
		Meta            Meta             `json:"meta"`
		Groups          []Group          `json:"groups"`
	} `json:"result"`
}
