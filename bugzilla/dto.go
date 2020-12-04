package bugzilla

import (
	"time"
)

type AssigneeResponse struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// BugResponse data model represents Bugzilla get bugsList results
type BugResponse struct {
	ID               int               `json:"id"`
	Product          string            `json:"product"`
	Component        string            `json:"component"`
	AssignedTo       *AssigneeResponse `json:"assigned_to"`
	ShortDescription string            `json:"short_description"`
	CreationTS       time.Time         `json:"creation_ts"`
	Priority         string            `json:"priority"`
	BugStatus        string            `json:"bug_status"`
	ChangedAt string  `json:"changed_at"`
	//Activity           []*BugActivityResponse `json:"activity"`
}

// todo: clean it if not used
// BugActivityResponse data model represents Bugzilla bugsActivity results
type BugActivityResponse struct {
	Added  string `json:"added"`
	What   string `json:"what"`
	Remove string `json:"remove"`
	Who    string `json:"who"`
	When   string `json:"when"`
}

// BugResponse data model represents Bugzilla get bugDetail results
type BugDetailResponse struct {
	Bug BugDetailXML `xml:"bug"`
}

// BugDetailXML ...
type BugDetailXML struct {
	ID         int    `xml:"bug_id"`
	CreationTS string `xml:"creation_ts"`
	Priority   string `xml:"priority"`
	Severity   string `xml:"bug_severity"`
	OpSys      string `xml:"op_sys"`
}

// SearchFields ...
type SearchFields struct {
	Component string `json:"component"`
	Product   string `json:"product"`
	ItemID    string `json:"item_id"`
}

// BugRaw data model represents es schema
type BugRaw struct {
	BackendVersion string `json:"backend_version"`
	BackendName    string `json:"backend_name"`
	UUID           string `json:"uuid"`
	BugID          int    `json:"bug_id"`
	Origin         string `json:"origin"`
	Tag            string `json:"tag"`
	Product        string `json:"product"`
	Component      string `json:"component"`
	Assignee       struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	ShortDescription  string    `json:"short_description"`
	BugStatus         string    `json:"bug_status"`
	MetadataUpdatedOn time.Time `json:"metadata__updated_on"`
	MetadataTimestamp time.Time `json:"metadata__timestamp"`
	Timestamp         float64   `json:"timestamp"`
	Category          string    `json:"category"`
	CreationTS        time.Time `json:"creation_ts"`
	Priority          string    `json:"priority"`
	Severity          string    `json:"severity"`
	OpSys             string    `json:"op_sys"`
	ChangedAt string `json:"changed_at"`
	//SearchFields             *SearchFields `json:"search_fields"`
}
