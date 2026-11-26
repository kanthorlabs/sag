package devx

type ACTest struct {
	Name  string
	Layer int
}

type ACRow struct {
	ID    string
	Run   string
	RunBy string
	Tests []ACTest
}

var ACTable = []ACRow{
	{
		ID:    "DX-AC01",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC01_Bootstrap", Layer: 2}},
	},
	{
		ID:    "DX-AC02",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC02_ContainerAndCert", Layer: 2}},
	},
	{
		ID:    "DX-AC03",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC03_BootstrapIdempotent", Layer: 3}},
	},
	{
		ID:    "DX-AC04",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC04_BootstrapNoTerminal", Layer: 2}},
	},
	{
		ID:    "DX-AC05",
		Run:   "B",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC05_PodmanAptInstall", Layer: 10}},
	},
	{
		ID:    "DX-AC06",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC06_InstallApproval", Layer: 3}},
	},
	{
		ID:    "DX-AC07",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC07_ReleaseChecksum", Layer: 3}},
	},
	{
		ID:    "DX-AC08",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC08_VersionRule", Layer: 3}},
	},
	{
		ID:    "DX-AC09",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC09_CreateCanary", Layer: 4}},
	},
	{
		ID:    "DX-AC10",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC10_CanaryNoLeak", Layer: 4}},
	},
	{
		ID:    "DX-AC11",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC11_CreateRejectsForm", Layer: 4}},
	},
	{
		ID:    "DX-AC12",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC12_ConnectionCheck", Layer: 4}},
	},
	{
		ID:    "DX-AC13",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC13_LoopbackWarn", Layer: 4}},
	},
	{
		ID:    "DX-AC14",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC14_CreateStdin", Layer: 4}},
	},
	{
		ID:    "DX-AC15",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC15_CreateSecretExists", Layer: 4}},
	},
	{
		ID:    "DX-AC16",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC16_CreateRollback", Layer: 4}},
	},
	{
		ID:    "DX-AC17",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC17_ListInspect", Layer: 4}},
	},
	{
		ID:    "DX-AC18",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC18_DeleteNoTerminal", Layer: 5}},
	},
	{
		ID:    "DX-AC19",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC19_DeleteConfirm", Layer: 5}},
	},
	{
		ID:    "DX-AC20",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC20_Delete", Layer: 5}},
	},
	{
		ID:    "DX-AC21",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC21_DeleteStopped", Layer: 5}},
	},
	{
		ID:    "DX-AC22",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC22_DeleteResume", Layer: 5}},
	},
	{
		ID:    "DX-AC23",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC23_DeleteForeignSecret", Layer: 5}},
	},
	{
		ID:    "DX-AC24",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC24_HostJWTGenerate", Layer: 5}},
	},
	{
		ID:    "DX-AC25",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC25_Logs", Layer: 5}},
	},
	{
		ID:    "DX-AC26",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC26_DenyRules", Layer: 6}},
	},
	{
		ID:    "DX-AC27",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC27_Autostart", Layer: 2}},
	},
	{
		ID:    "DX-AC28",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC28_ServiceStopStart", Layer: 2}},
	},
	{
		ID:    "DX-AC29",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC29_CertRenewal", Layer: 4}},
	},
	{
		ID:    "DX-AC30",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC30_ImageSelection", Layer: 7}},
	},
	{
		ID:    "DX-AC31",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC31_AWSBackend", Layer: 8}},
	},
	{
		ID:    "DX-AC32",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC32_AWSUnavailable", Layer: 8}},
	},
	{
		ID:    "DX-AC33",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC33_ProductBinary", Layer: 2}},
	},
	{
		ID:    "DX-AC34",
		Run:   "A",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC34_MacBootstrap", Layer: 9}},
	},
	{
		ID:    "DX-AC35",
		Run:   "A",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC35_MacCredential", Layer: 9}},
	},
	{
		ID:    "DX-AC36",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC36_HostGuards", Layer: 2}},
	},
	{
		ID:    "DX-AC37",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC37_CreateTLSCA", Layer: 4}},
	},
	{
		ID:    "DX-AC38",
		Run:   "A",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC38_MacPkgChecksum", Layer: 9}},
	},
	{
		ID:    "DX-AC39",
		Run:   "AB",
		RunBy: "both",
		Tests: []ACTest{
			{Name: "DXAC39_ClaudeCodeSetup", Layer: 6},
			{Name: "DXAC39_DefaultPath", Layer: 9},
		},
	},
	{
		ID:    "DX-AC40",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC40_JWTNoLeak", Layer: 6}},
	},
	{
		ID:    "DX-AC41",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC41_PiSetup", Layer: 6}},
	},
	{
		ID:    "DX-AC42",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC42_CATrustConflict", Layer: 6}},
	},
	{
		ID:    "DX-AC43",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC43_AgentSetupRefusals", Layer: 6}},
	},
	{
		ID:    "DX-AC44",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC44_SagGuard", Layer: 6}},
	},
	{
		ID:    "DX-AC45",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC45_AgentIssue", Layer: 6}},
	},
	{
		ID:    "DX-AC46",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC46_BootstrapAgentSetup", Layer: 6}},
	},
	{
		ID:    "DX-AC47",
		Run:   "A",
		RunBy: "ulrich",
		Tests: []ACTest{{Name: "DXAC47_RealClaudeCode", Layer: 9}},
	},
	{
		ID:    "DX-AC48",
		Run:   "A",
		RunBy: "ulrich",
		Tests: []ACTest{{Name: "DXAC48_RealPi", Layer: 9}},
	},
	{
		ID:    "DX-AC49",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "TestDXAC49_SkillNamesToolsAndCodes", Layer: 6}},
	},
	{
		ID:    "DX-AC50",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC50_TailscaleFake", Layer: 3}},
	},
	{
		ID:    "DX-AC51",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC51_RemoteMode", Layer: 6}},
	},
	{
		ID:    "DX-AC52",
		Run:   "A",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC52_MacTailscale", Layer: 9}},
	},
	{
		ID:    "DX-AC53",
		Run:   "AB",
		RunBy: "agent",
		Tests: []ACTest{{Name: "DXAC53_PortHolder", Layer: 3}},
	},
}
