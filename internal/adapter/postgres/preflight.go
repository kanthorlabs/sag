package postgres

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/kanthorlabs/sag/internal/approval"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/sql"
)

const (
	checkEngine        = "engine"
	checkVersion       = "version"
	checkReadOnlyMode  = "read_only_mode"
	checkRiskyObjects  = "risky_objects"
	checkOperatorNames = "operator_names"
	enginePrefix       = "PostgreSQL "
)

const (
	minVersionNum = 140000
	maxVersionNum = 199999
)

const (
	querySearchPath         = `SELECT pg_catalog.set_config('search_path', 'pg_catalog', true)`
	queryServer             = `SELECT pg_catalog.version(), pg_catalog.current_setting('server_version_num')`
	queryTLS                = `SELECT s.ssl FROM pg_catalog.pg_stat_ssl s WHERE s.pid = pg_catalog.pg_backend_pid()`
	queryReadOnly           = `SELECT pg_catalog.current_setting('transaction_read_only'), pg_catalog.current_setting('standard_conforming_strings'), pg_catalog.current_setting('client_encoding')`
	queryRiskyExtensions    = `SELECT e.extname, e.extversion, n.nspname FROM pg_catalog.pg_extension e JOIN pg_catalog.pg_namespace n ON n.oid = e.extnamespace WHERE e.extname IN ('dblink', 'pg_background', 'adminpack', 'file_fdw')`
	queryUntrustedLanguages = `SELECT l.lanname FROM pg_catalog.pg_language l WHERE NOT l.lanpltrusted AND l.lanname NOT IN ('c', 'internal')`
)

const (
	detailVersionOutOfRange = "server_version_num %s is outside 140000 to 199999"
	detailReadOnlyMode      = "transaction_read_only %s, standard_conforming_strings %s, client_encoding %s"
	detailNoRiskyObjects    = "no risky objects"
	detailApproved          = "approved: %s"
	detailNotApproved       = "not approved: %s"
	detailApprovalInvalid   = "approval record is not valid for the current objects: %s"
	detailNoOperators       = "none"
	approvalHint            = "resource %s has risky objects %s; to approve, run on the host: podman exec -it sag sag extension approve %s"
)

var errCatalogValue = errors.New("unexpected catalog value")

func (d *dialect) Preflight(ctx context.Context, conn sql.CatalogReader) (core.PreflightReport, error) {
	report, err := d.preflight(ctx, conn)
	if err != nil {
		return core.PreflightReport{}, preflightError(ctx, preflightQueryFailed, err)
	}
	report.Passed = !slices.ContainsFunc(report.Checks, func(c core.PreflightCheck) bool { return !c.Passed })
	return report, nil
}

func (d *dialect) preflight(ctx context.Context, r sql.CatalogReader) (core.PreflightReport, error) {
	var report core.PreflightReport
	if err := pinSearchPath(ctx, r); err != nil {
		return report, err
	}
	engine, version, err := checkServer(ctx, r)
	if err != nil {
		return report, err
	}
	report.Checks = append(report.Checks, engine)
	if !engine.Passed {
		return report, nil
	}
	report.Checks = append(report.Checks, version)
	if !version.Passed {
		return report, nil
	}
	sessionSSL, err := readSessionSSL(ctx, r)
	if err != nil {
		return report, err
	}
	tls, warning := checkTLS(d.cfg.TLS, d.target.Host, sessionSSL)
	report.Checks = append(report.Checks, tls)
	if warning != "" {
		report.Warnings = append(report.Warnings, warning)
	}
	readOnly, err := checkReadOnly(ctx, r)
	if err != nil {
		return report, err
	}
	report.Checks = append(report.Checks, readOnly)
	risky, hint, err := d.checkRisky(ctx, r)
	if err != nil {
		return report, err
	}
	report.Checks = append(report.Checks, risky)
	report.Hint = hint
	names, err := operatorCollisions(ctx, r)
	if err != nil {
		return report, err
	}
	report.Checks = append(report.Checks, operatorNamesCheck(names))
	return report, nil
}

func pinSearchPath(ctx context.Context, r sql.CatalogReader) error {
	_, err := queryOne(ctx, r, querySearchPath)
	return err
}

func checkServer(ctx context.Context, r sql.CatalogReader) (core.PreflightCheck, core.PreflightCheck, error) {
	row, err := queryOne(ctx, r, queryServer)
	if err != nil {
		return core.PreflightCheck{}, core.PreflightCheck{}, err
	}
	version, err := textColumn(row, 0)
	if err != nil {
		return core.PreflightCheck{}, core.PreflightCheck{}, err
	}
	num, err := textColumn(row, 1)
	if err != nil {
		return core.PreflightCheck{}, core.PreflightCheck{}, err
	}
	engine := core.PreflightCheck{Name: checkEngine, Passed: strings.HasPrefix(version, enginePrefix), Detail: version}
	n, err := strconv.Atoi(num)
	inRange := err == nil && n >= minVersionNum && n <= maxVersionNum
	detail := num
	if !inRange {
		detail = fmt.Sprintf(detailVersionOutOfRange, strconv.Quote(num))
	}
	return engine, core.PreflightCheck{Name: checkVersion, Passed: inRange, Detail: detail}, nil
}

func readSessionSSL(ctx context.Context, r sql.CatalogReader) (bool, error) {
	rows, err := queryRows(ctx, r, queryTLS)
	if err != nil || len(rows) != 1 || len(rows[0]) != 1 {
		return false, err
	}
	ssl, ok := rows[0][0].(bool)
	return ok && ssl, nil
}

func checkReadOnly(ctx context.Context, r sql.CatalogReader) (core.PreflightCheck, error) {
	row, err := queryOne(ctx, r, queryReadOnly)
	if err != nil {
		return core.PreflightCheck{}, err
	}
	values := make([]string, 3)
	for i := range values {
		if values[i], err = textColumn(row, i); err != nil {
			return core.PreflightCheck{}, err
		}
	}
	passed := values[0] == "on" && values[1] == "on" && values[2] == "UTF8"
	return core.PreflightCheck{Name: checkReadOnlyMode, Passed: passed, Detail: fmt.Sprintf(detailReadOnlyMode, values[0], values[1], values[2])}, nil
}

func (d *dialect) checkRisky(ctx context.Context, r sql.CatalogReader) (core.PreflightCheck, string, error) {
	objects, err := riskyObjects(ctx, r)
	if err != nil {
		return core.PreflightCheck{}, "", err
	}
	names := objectNames(objects)
	if len(names) == 0 {
		return core.PreflightCheck{Name: checkRiskyObjects, Passed: true, Detail: detailNoRiskyObjects}, "", nil
	}
	list := strings.Join(names, ", ")
	hint := fmt.Sprintf(approvalHint, d.cfg.Name, list, d.cfg.Name)
	refused := core.PreflightCheck{Name: checkRiskyObjects, Passed: false, Detail: fmt.Sprintf(detailNotApproved, list)}
	if d.cfg.Approvals == nil {
		return refused, hint, nil
	}
	fingerprint, err := approval.Fingerprint(d.target, objects)
	if err != nil {
		return refused, hint, nil
	}
	if err := d.cfg.Approvals.Verify(d.cfg.Name, fingerprint); err != nil {
		if !errors.Is(err, approval.ErrNotApproved) {
			refused.Detail = fmt.Sprintf(detailApprovalInvalid, list)
		}
		return refused, hint, nil
	}
	return core.PreflightCheck{Name: checkRiskyObjects, Passed: true, Detail: fmt.Sprintf(detailApproved, list)}, "", nil
}

func riskyObjects(ctx context.Context, r sql.CatalogReader) (approval.Objects, error) {
	var objects approval.Objects
	rows, err := queryRows(ctx, r, queryRiskyExtensions)
	if err != nil {
		return approval.Objects{}, err
	}
	for _, row := range rows {
		var e approval.Extension
		for i, field := range []*string{&e.Name, &e.Version, &e.Schema} {
			if *field, err = textColumn(row, i); err != nil {
				return approval.Objects{}, err
			}
		}
		objects.Extensions = append(objects.Extensions, e)
	}
	rows, err = queryRows(ctx, r, queryUntrustedLanguages)
	if err != nil {
		return approval.Objects{}, err
	}
	for _, row := range rows {
		name, err := textColumn(row, 0)
		if err != nil {
			return approval.Objects{}, err
		}
		objects.Languages = append(objects.Languages, name)
	}
	return objects, nil
}

func objectNames(o approval.Objects) []string {
	extensions := make([]string, 0, len(o.Extensions))
	for _, e := range o.Extensions {
		extensions = append(extensions, e.Name)
	}
	slices.Sort(extensions)
	languages := slices.Sorted(slices.Values(o.Languages))
	return append(extensions, languages...)
}

func operatorNamesCheck(names []string) core.PreflightCheck {
	detail := detailNoOperators
	if len(names) > 0 {
		detail = strings.Join(names, " ")
	}
	return core.PreflightCheck{Name: checkOperatorNames, Passed: true, Detail: detail}
}

func queryRows(ctx context.Context, r sql.CatalogReader, text string) ([][]any, error) {
	rows, err := r.Query(ctx, text)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]any
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		out = append(out, values)
	}
	rows.Close()
	return out, rows.Err()
}

func queryOne(ctx context.Context, r sql.CatalogReader, text string) ([]any, error) {
	rows, err := queryRows(ctx, r, text)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, errCatalogValue
	}
	return rows[0], nil
}

func textColumn(row []any, i int) (string, error) {
	if i >= len(row) {
		return "", errCatalogValue
	}
	s, ok := row[i].(string)
	if !ok {
		return "", errCatalogValue
	}
	return s, nil
}
