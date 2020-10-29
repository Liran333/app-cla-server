package mongodb

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/opensourceways/app-cla-server/dbmodels"
	"github.com/opensourceways/app-cla-server/util"
)

type corporationManagerDoc struct {
	Name             string `bson:"name" json:"name" required:"true"`
	Role             string `bson:"role" json:"role" required:"true"`
	Email            string `bson:"email"  json:"email" required:"true"`
	Password         string `bson:"password" json:"password" required:"true"`
	InitialPWChanged bool   `bson:"changed" json:"changed"`
}

func corpManagerField(field string) string {
	return fmt.Sprintf("%s.%s", fieldCorpoManagers, field)
}

func filterForCorpManager(filter bson.M) {
	filter["apply_to"] = dbmodels.ApplyToCorporation
	filter["enabled"] = true
	filter[fieldCorpoManagers] = bson.M{"$type": "array"}
}

func managersToAdd(
	ctx context.Context, c *client, oid primitive.ObjectID,
	opt []dbmodels.CorporationManagerCreateOption, managerNumber int,
) ([]dbmodels.CorporationManagerCreateOption, error) {

	ms, err := c.listCorporationManager(ctx, oid, opt[0].Email, opt[0].Role)
	if err != nil {
		return nil, err
	}

	current := map[string]bool{}
	for _, item := range ms {
		current[item.Email] = true
	}

	toAdd := make([]dbmodels.CorporationManagerCreateOption, 0, len(opt))
	for _, item := range opt {
		if _, ok := current[item.Email]; !ok {
			toAdd = append(toAdd, item)
		}
	}

	if len(ms)+len(toAdd) > managerNumber {
		return nil, dbmodels.DBError{
			ErrCode: util.ErrNumOfCorpManagersExceeded,
			Err:     fmt.Errorf("exceed %d managers allowed", managerNumber),
		}
	}

	return toAdd, nil
}

func (c *client) AddCorporationManager(orgCLAID string, opt []dbmodels.CorporationManagerCreateOption, managerNumber int) ([]dbmodels.CorporationManagerCreateOption, error) {
	oid, err := toObjectID(orgCLAID)
	if err != nil {
		return nil, err
	}

	var toAdd []dbmodels.CorporationManagerCreateOption

	f := func(ctx mongo.SessionContext) error {
		toAdd, err = managersToAdd(ctx, c, oid, opt, managerNumber)
		if err != nil {
			return err
		}
		if len(toAdd) == 0 {
			return nil
		}

		items := make(bson.A, 0, len(toAdd))
		for _, item := range toAdd {
			info := corporationManagerDoc{
				Name:     item.Name,
				Email:    item.Email,
				Role:     item.Role,
				Password: item.Password,
			}

			body, err := structToMap(info)
			if err != nil {
				return err
			}
			addCorporationID(item.Email, body)

			items = append(items, body)
		}

		err = c.pushArrayElems(
			ctx, orgCLACollection, fieldCorpoManagers,
			filterOfDocID(oid), items,
		)
		if err != nil {
			return err
		}

		if opt[0].Role == dbmodels.RoleAdmin {
			return c.updateArrayElem(
				ctx, orgCLACollection, fieldCorporations,
				filterOfDocID(oid),
				filterOfCorpID(opt[0].Email),
				bson.M{"admin_added": true},
				true,
			)
		}
		return nil
	}

	err = c.doTransaction(f)
	return toAdd, err
}

func (c *client) CheckCorporationManagerExist(opt dbmodels.CorporationManagerCheckInfo) (map[string]dbmodels.CorporationManagerCheckResult, error) {
	filterOfDoc := bson.M{}
	filterForCorpManager(filterOfDoc)

	filterOfArray := indexOfCorpManagerAndIndividual(opt.User)
	filterOfArray["password"] = opt.Password

	project := bson.M{
		"platform":                  1,
		"org_id":                    1,
		fieldRepo:                   1,
		corpManagerField("role"):    1,
		corpManagerField("name"):    1,
		corpManagerField("email"):   1,
		corpManagerField("changed"): 1,
	}

	var v []OrgCLA

	f := func(ctx context.Context) error {
		return c.getArrayElem(ctx, orgCLACollection, fieldCorpoManagers, filterOfDoc, filterOfArray, project, &v)
	}

	if err := withContext(f); err != nil {
		return nil, err
	}

	if len(v) == 0 {
		return nil, dbmodels.DBError{
			ErrCode: util.ErrNoDBRecord,
			Err:     fmt.Errorf("no cla binding found"),
		}
	}

	result := map[string]dbmodels.CorporationManagerCheckResult{}
	for _, doc := range v {
		cm := doc.CorporationManagers
		if len(cm) == 0 {
			continue
		}

		item := &cm[0]
		result[objectIDToUID(doc.ID)] = dbmodels.CorporationManagerCheckResult{
			Email:            item.Email,
			Role:             item.Role,
			Platform:         doc.Platform,
			OrgID:            doc.OrgID,
			RepoID:           toNormalRepo(doc.RepoID),
			InitialPWChanged: item.InitialPWChanged,
		}
	}
	return result, nil
}

func (c *client) ResetCorporationManagerPassword(orgCLAID, email string, opt dbmodels.CorporationManagerResetPassword) error {
	oid, err := toObjectID(orgCLAID)
	if err != nil {
		return err
	}

	updateCmd := bson.M{
		"password": opt.NewPassword,
		"changed":  true,
	}

	filterOfArray := indexOfCorpManagerAndIndividual(email)
	filterOfArray["password"] = opt.OldPassword

	f := func(ctx context.Context) error {
		return c.updateArrayElem(ctx, orgCLACollection, fieldCorpoManagers, filterOfDocID(oid), filterOfArray, updateCmd, true)
	}

	return withContext(f)
}

func (c *client) listCorporationManager(ctx context.Context, orgCLAID primitive.ObjectID, email, role string) ([]corporationManagerDoc, error) {
	filterOfArray := filterOfCorpID(email)
	if role != "" {
		filterOfArray["role"] = role
	}

	project := bson.M{
		corpManagerField("name"):  1,
		corpManagerField("email"): 1,
		corpManagerField("role"):  1,
	}

	var v []OrgCLA
	err := c.getArrayElem(
		ctx, orgCLACollection, fieldCorpoManagers,
		filterOfDocID(orgCLAID), filterOfArray, project, &v,
	)
	if err != nil {
		return nil, err
	}

	if len(v) == 0 {
		return nil, dbmodels.DBError{
			ErrCode: util.ErrNoDBRecord,
			Err:     fmt.Errorf("can't find the cla"),
		}
	}
	return v[0].CorporationManagers, nil
}

func (c *client) ListCorporationManager(orgCLAID, email, role string) ([]dbmodels.CorporationManagerListResult, error) {
	oid, err := toObjectID(orgCLAID)
	if err != nil {
		return nil, err
	}

	var v []corporationManagerDoc

	f := func(ctx context.Context) error {
		r, err := c.listCorporationManager(ctx, oid, email, role)
		v = r
		return err
	}

	if err = withContext(f); err != nil {
		return nil, err
	}

	ms := make([]dbmodels.CorporationManagerListResult, 0, len(v))
	for _, item := range v {
		ms = append(ms, dbmodels.CorporationManagerListResult{
			Name:  item.Name,
			Email: item.Email,
			Role:  item.Role,
		})
	}
	return ms, nil
}

func (c *client) DeleteCorporationManager(orgCLAID, role string, emails []string) ([]dbmodels.CorporationManagerCreateOption, error) {
	oid, err := toObjectID(orgCLAID)
	if err != nil {
		return nil, err
	}

	deleted := make([]dbmodels.CorporationManagerCreateOption, 0, len(emails))

	f := func(ctx mongo.SessionContext) error {
		ms, err := c.listCorporationManager(ctx, oid, emails[0], role)
		if err != nil {
			return err
		}

		all := map[string]int{}
		for i, item := range ms {
			all[item.Email] = i
		}

		toDelete := make(bson.A, 0, len(emails))
		for _, email := range emails {
			if i, ok := all[email]; ok {
				toDelete = append(toDelete, email)
				deleted = append(deleted, dbmodels.CorporationManagerCreateOption{
					Email: ms[i].Email,
					Name:  ms[i].Name,
				})
			}
		}
		if len(toDelete) == 0 {
			return nil
		}

		filterOfArray := filterOfCorpID(emails[0])
		filterOfArray["email"] = bson.M{"$in": toDelete}

		return c.pullArrayElem(
			ctx, orgCLACollection, fieldCorpoManagers,
			filterOfDocID(oid), filterOfArray,
		)
	}

	err = c.doTransaction(f)
	return deleted, err
}
