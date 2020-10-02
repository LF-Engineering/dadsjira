package dads

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const (
	// MultiOrgNames - suffix for multiple orgs affiliation data
	MultiOrgNames = "_multi_org_names"
)

var (
	identityCache = map[string][2]interface{}{}
	rollsCache    = map[string][]string{}
)

// EmptyAffsItem - return empty affiliation sitem for a given role
func EmptyAffsItem(role string, undef bool) map[string]interface{} {
	emp := ""
	if undef {
		emp = "-- UNDEFINED --"
	}
	return map[string]interface{}{
		role + "_id":         emp,
		role + "_uuid":       emp,
		role + "_name":       emp,
		role + "_user_name":  emp,
		role + "_domain":     emp,
		role + "_gender":     emp,
		role + "_gender_acc": nil,
		role + "_org_name":   emp,
		role + "_bot":        false,
		role + MultiOrgNames: []interface{}{},
	}
}

// IdentityAffsDomain -return domain for given identity using email if specified
func IdentityAffsDomain(identity map[string]interface{}) (domain interface{}) {
	// domain = self.get_email_domain(identity['email'])
	email, ok := identity["email"].(string)
	if ok {
		ary := strings.Split(email, "@")
		if len(ary) > 1 {
			domain = ary[1]
		}
	}
	return
}

// FindObject - fetch given fields from object (identities, profiles, uidentities etc.) having key=id
// Assuming that given object has an unique key to gte it
func FindObject(ctx *Ctx, object, key, id string, fields []string) (obj map[string]interface{}, err error) {
	var rows *sql.Rows
	rows, err = QuerySQL(ctx, nil, fmt.Sprintf("select %s from %s where %s = ? limit 1", strings.Join(fields, ", "), object, key), id)
	if err != nil {
		return
	}
	for rows.Next() {
		obj = make(map[string]interface{})
		data := make([]interface{}, len(fields))
		for i := range data {
			data[i] = new(interface{})
		}
		err = rows.Scan(data...)
		if err != nil {
			return
		}
		for i, val := range data {
			v := *val.(*interface{})
			if v == nil {
				obj[fields[i]] = v
				continue
			}
			switch cV := v.(type) {
			case []byte:
				obj[fields[i]] = string(cV)
			default:
				obj[fields[i]] = cV
			}
		}
		break
	}
	err = rows.Err()
	if err != nil {
		return
	}
	err = rows.Close()
	return
}

// AffsIdentityIDs - returns affiliations identity id, uuid data
// uses internal cache
func AffsIdentityIDs(ctx *Ctx, ds DS, identity map[string]interface{}) (ids [2]interface{}) {
	email, _ := identity["email"]
	name, _ := identity["name"]
	username, _ := identity["username"]
	if email == nil && name == nil && username == nil {
		return
	}
	sEmail, okE := email.(string)
	sName, okN := name.(string)
	sUsername, okU := username.(string)
	k := sEmail + ":" + sName + ":" + sUsername
	ids, ok := identityCache[k]
	if ok {
		return
	}
	if !okE {
		sEmail = Nil
	}
	if !okN {
		sName = Nil
	}
	if !okU {
		sUsername = Nil
	}
	source := ds.Name()
	id := UUIDAffs(ctx, source, sEmail, sName, sUsername)
	identityFound, err := FindObject(ctx, "identities", "id", id, []string{"id", "uuid"})
	if err != nil || identityFound == nil {
		identityCache[k] = ids
		return
	}
	ids[0] = identityFound["id"]
	ids[1] = identityFound["uuid"]
	identityCache[k] = ids
	return
}

// GetEnrollments - returns enrollments for a given uuid in a given date, possibly multiple
// uses cache with date resolution (uuid,dt.YYYYMMDD)
func GetEnrollments(ctx *Ctx, ds DS, uuid string, dt time.Time, single bool) (orgs []string) {
	sSep := "m"
	if single {
		sSep = "s"
	}
	k := uuid + sSep + ToYMDDate(dt)
	orgs, ok := rollsCache[k]
	if ok {
		return
	}
	defer func() {
		rollsCache[k] = orgs
	}()
	return
}

// GetEnrollmentsSingle - returns org name (or Unknown) for given uuid and date
func GetEnrollmentsSingle(ctx *Ctx, ds DS, uuid string, dt time.Time) (org string) {
	orgs := GetEnrollments(ctx, ds, uuid, dt, true)
	if len(orgs) == 0 {
		org = Unknown
		return
	}
	org = orgs[0]
	return
}

// GetEnrollmentsMulti - returns org name(s) for given uuid and name
// Returns 1 or more organizations (all that matches the current date)
// If none matches it returns array [Unknown]
func GetEnrollmentsMulti(ctx *Ctx, ds DS, uuid string, dt time.Time) (orgs []string) {
	orgs = GetEnrollments(ctx, ds, uuid, dt, false)
	if len(orgs) == 0 {
		orgs = append(orgs, Unknown)
	}
	return
}

// IdenityAffsData - add affiliations related data
func IdenityAffsData(ctx *Ctx, ds DS, identity map[string]interface{}, dt time.Time, role string) (outItem map[string]interface{}) {
	// FIXME: possibly needs to add AffID support
	// enrich 764
	ids := AffsIdentityIDs(ctx, ds, identity)
	outItem = EmptyAffsItem(role, false)
	outItem[role+"_id"] = ids[0]
	outItem[role+"_uuid"] = ids[1]
	name, _ := identity["name"]
	if name == nil {
		outItem[role+"_name"] = ""
	} else {
		outItem[role+"_name"] = name
	}
	username, _ := identity["username"]
	if username == nil {
		outItem[role+"_user_name"] = ""
	} else {
		outItem[role+"_user_name"] = username
	}
	outItem[role+"_domain"] = IdentityAffsDomain(identity)
	uuid := ids[1]
	if uuid == nil {
		outItem = EmptyAffsItem(role, true)
		return
	}
	suuid, _ := uuid.(string)
	profile, err := FindObject(ctx, "profiles", "uuid", suuid, []string{"name", "email", "gender", "gender_acc", "is_bot"})
	isBot := 0
	if err == nil && profile != nil {
		pName, _ := profile["name"]
		if pName != nil {
			outItem[role+"_name"] = pName
		}
		email, _ := profile["email"]
		if email != nil {
			ary := strings.Split(email.(string), "@")
			if len(ary) > 1 {
				outItem[role+"_domain"] = ary[1]
			}
		}
		gender, _ := profile["gender"]
		if gender != nil {
			outItem[role+"_gender"] = gender
		} else {
			outItem[role+"_gender"] = Unknown
		}
		bot, ok := profile["is_bot"].(int64)
		if ok && bot > 0 {
			isBot = 1
		}
	}
	gender, ok := outItem[role+"_gender"]
	if !ok || gender == nil {
		outItem[role+"_gender"] = Unknown
		outItem[role+"_gender_acc"] = 0
	}
	outItem[role+"_bot"] = isBot
	outItem[role+"_org_name"] = GetEnrollmentsSingle(ctx, ds, suuid, dt)
	outItem[role+MultiOrgNames] = GetEnrollmentsMulti(ctx, ds, suuid, dt)
	Printf("identity=%+v, ids=%+v, profile=%+v outItem=%+v\n", identity, ids, profile, outItem)
	return
}
