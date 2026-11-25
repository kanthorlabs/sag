package e2e

type ACRow struct {
	ID    string
	Layer int
	Test  string
}

var ACTable = []ACRow{
	{ID: "AC01", Layer: 1, Test: "AC01_ConfigCheckRejects"},
	{ID: "AC02", Layer: 1, Test: "AC02_StartRefusesBadSecrets"},
	{ID: "AC03", Layer: 1, Test: "AC03_JWTRejects"},
	{ID: "AC04", Layer: 1, Test: "AC04_JWTGenerateRefuses"},
	{ID: "AC05", Layer: 2, Test: "AC05_C2BlocksWrites"},
	{ID: "AC06", Layer: 2, Test: "AC06_SingleStatement"},
	{ID: "AC07", Layer: 2, Test: "AC07_NoPassThrough"},
	{ID: "AC08", Layer: 2, Test: "AC08_RollbackOnly"},
	{ID: "AC09", Layer: 2, Test: "AC09_RefusesPG13"},
	{ID: "AC10", Layer: 2, Test: "AC10_TLSVerifyFull"},
	{ID: "AC11", Layer: 2, Test: "AC11_ExtensionApproval"},
	{ID: "AC12", Layer: 2, Test: "AC12_OperatorCollision"},
	{ID: "AC13", Layer: 3, Test: "AC13_StaticRejects"},
	{ID: "AC14", Layer: 3, Test: "AC14_FieldAllowlist"},
	{ID: "AC15", Layer: 3, Test: "AC15_C1OnlyEffects"},
	{ID: "AC16", Layer: 3, Test: "AC16_LexerSettings"},
	{ID: "AC17", Layer: 3, Test: "AC17_RenderedTextOnly"},
	{ID: "AC18", Layer: 3, Test: "AC18_ViewWriteBlocked"},
	{ID: "AC19", Layer: 3, Test: "AC19_Integrity"},
	{ID: "AC20", Layer: 4, Test: "AC20_MaxRows"},
	{ID: "AC21", Layer: 4, Test: "AC21_MaxResultBytes"},
	{ID: "AC22", Layer: 4, Test: "AC22_LockTimeout"},
	{ID: "AC23", Layer: 4, Test: "AC23_RateLimit"},
	{ID: "AC24", Layer: 4, Test: "AC24_ConnectErrorScrub"},
	{ID: "AC25", Layer: 4, Test: "AC25_ColumnRedaction"},
	{ID: "AC26", Layer: 4, Test: "AC26_AuditRecord"},
	{ID: "AC27", Layer: 4, Test: "AC27_AuditFailure"},
	{ID: "AC28", Layer: 5, Test: "AC28_EndpointAuthOrigin"},
	{ID: "AC29", Layer: 5, Test: "AC29_Authorization"},
	{ID: "AC30", Layer: 5, Test: "AC30_ToolShapes"},
	{ID: "AC31", Layer: 5, Test: "AC31_ClaudeCode"},
	{ID: "AC32", Layer: 6, Test: "AC32_ContainerInspect"},
	{ID: "AC33", Layer: 6, Test: "AC33_SecretMounts"},
	{ID: "AC34", Layer: 6, Test: "AC34_JWTGenerateNoTTY"},
	{ID: "AC35", Layer: 6, Test: "AC35_CI"},
}
