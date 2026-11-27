package postgres

import (
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var credentialCatalogs = map[string]bool{
	"pg_authid":             true,
	"pg_shadow":             true,
	"pg_user_mapping":       true,
	"pg_user_mappings":      true,
	"pg_subscription":       true,
	"pg_settings":           true,
	"pg_file_settings":      true,
	"pg_statistic":          true,
	"pg_stats":              true,
	"pg_statistic_ext_data": true,
	"pg_stats_ext":          true,
	"pg_stats_ext_exprs":    true,
	"user_mapping_options":  true,
	"_pg_user_mappings":     true,
}

func checkRelations(t *pgTree) error {
	return eachMessage(t.Result.ProtoReflect(), func(m protoreflect.Message) error {
		rv, ok := m.Interface().(*pg_query.RangeVar)
		if !ok {
			return nil
		}
		name := qualifiedName{Schema: rv.GetSchemaname(), Name: rv.GetRelname()}
		if rv.GetCatalogname() != "" {
			return &Rejection{Rule: ruleNameParts, Name: rv.GetCatalogname() + "." + name.String()}
		}
		if credentialCatalogs[strings.ToLower(name.Name)] {
			return &Rejection{Rule: ruleRelation, Name: name.String()}
		}
		return nil
	})
}
