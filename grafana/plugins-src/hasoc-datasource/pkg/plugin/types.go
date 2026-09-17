package plugin

// posture is ha_soc/risk/posture's result, matching RiskEngine.async_compute_posture
// in trooperthorn/ha_int_soc's risk.py.
type posture struct {
	Score        int            `json:"score"`
	Grade        string         `json:"grade"`
	Provisional  bool           `json:"provisional"`
	MissingTerms []string       `json:"missing_terms"`
	Breakdown    postureFactors `json:"breakdown"`
}

type postureFactors struct {
	PUser        float64 `json:"p_user"`
	PVuln        float64 `json:"p_vuln"`
	PMisconfig   float64 `json:"p_misconfig"`
	PIntegration float64 `json:"p_integration"`
	PDetection   float64 `json:"p_detection"`
}

// userRisk is one entry of ha_soc/risk/list's result, matching
// RiskEngine._compute_user_risk's return shape.
type userRisk struct {
	UserID  string       `json:"user_id"`
	Score   int          `json:"score"`
	Band    string       `json:"band"`
	Factors []riskFactor `json:"factors"`
}

type riskFactor struct {
	Name   string `json:"name"`
	Points int    `json:"points"`
	Detail string `json:"detail"`
}

// topFactor returns the highest-points factor's name, or "" when there are
// none (factors already arrive points-descending from _compute_user_risk).
func (u userRisk) topFactor() string {
	if len(u.Factors) == 0 {
		return ""
	}
	return u.Factors[0].Name
}

// auditEvent is one ha_soc/audit/query record, matching AuditLog.async_log's
// record shape in trooperthorn/ha_int_soc's audit.py.
type auditEvent struct {
	TS        string   `json:"ts"`
	UserID    string   `json:"user_id"`
	Category  string   `json:"category"`
	Domain    string   `json:"domain"`
	Service   string   `json:"service"`
	EntityIDs []string `json:"entity_ids"`
	IP        string   `json:"ip"`
	Seq       int64    `json:"seq"`
}
