package postgres

const (
	ruleStatementCount   = "statement_count"
	ruleStatement        = "statement"
	ruleExplainOption    = "explain_option"
	ruleNode             = "node"
	ruleNameParts        = "name_parts"
	ruleRelation         = "relation"
	ruleFunctionSchema   = "function_schema"
	ruleFunctionPrefix   = "function_prefix"
	ruleFunctionDenied   = "function_denied"
	ruleFunctionCatalog  = "function_catalog"
	ruleFunctionVolatile = "function_volatile"
	ruleOperatorSchema   = "operator_schema"
	ruleOperatorCatalog  = "operator_catalog"
	ruleRender           = "render"
	ruleDepth            = "depth"
)

const rejectionPrefix = "statement rejected: "

type Rejection struct {
	Node  string
	Field string
	Name  string
	Rule  string
}

func (r *Rejection) Reason() string {
	if r.Rule != "" {
		return r.Rule + ":" + r.Name
	}
	return r.Node + "." + r.Field
}

var closedDetailRules = map[string]bool{
	ruleStatementCount: true,
	ruleStatement:      true,
	ruleNode:           true,
	ruleDepth:          true,
}

func (r *Rejection) Message() string {
	if r.Rule != "" && !closedDetailRules[r.Rule] {
		return rejectionPrefix + r.Rule
	}
	return rejectionPrefix + r.Reason()
}

func (r *Rejection) Error() string {
	return rejectionPrefix + r.Reason()
}
