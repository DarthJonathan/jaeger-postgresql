package pgstore

import (
	"context"
	"time"

	"github.com/go-pg/pg/v9"

	hclog "github.com/hashicorp/go-hclog"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var _ spanstore.Reader = (*Reader)(nil)

// Reader can query for and load traces from PostgreSQL v2.x.
type Reader struct {
	db *pg.DB

	logger hclog.Logger
}

// NewReader returns a new SpanReader for PostgreSQL v2.x.
func NewReader(db *pg.DB, logger hclog.Logger) *Reader {
	return &Reader{
		db:     db,
		logger: logger,
	}
}

// GetServices returns all services traced by Jaeger
func (r *Reader) GetServices(ctx context.Context) ([]string, error) {
	r.logger.Debug("GetServices called")

	var services []Service
	err := r.db.Model(&services).Order("service_name ASC").Select()
	ret := make([]string, 0, len(services))

	for _, service := range services {
		if len(service.ServiceName) > 0 {
			ret = append(ret, service.ServiceName)
		}
	}

	return ret, err
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (r *Reader) GetOperations(ctx context.Context, param spanstore.OperationQueryParameters) ([]spanstore.Operation, error) {
	var operations []Operation
	err := r.db.Model(&operations).Order("operation_name ASC").Select()
	ret := make([]spanstore.Operation, 0, len(operations))
	for _, operation := range operations {
		if len(operation.OperationName) > 0 {
			ret = append(ret, spanstore.Operation{Name: operation.OperationName})
		}
	}

	return ret, err
}

// GetTrace takes a traceID and returns a Trace associated with that traceID
func (r *Reader) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	r.logger.Debug("GetTrace called for traceId ", traceID.String())

	builder := &whereBuilder{where: "", params: make([]interface{}, 0)}

	if traceID.Low > 0 {
		builder.andWhere(traceID.Low, "trace_id_low = ?")
	}
	if traceID.High > 0 {
		builder.andWhere(traceID.Low, "trace_id_high = ?")
	}

	var spans []Span
	err := r.db.Model(&spans).Where(builder.where, builder.params...).Limit(1).Select()
	ret := make([]*model.Span, 0, len(spans))
	ret2 := make([]model.Trace_ProcessMapping, 0, len(spans))
	for _, span := range spans {
		ret = append(ret, toModelSpan(span))
		modelProcess := model.Process{
			Tags: mapToModelKV(span.ProcessTags),
		}

		if span.Service != nil {
			modelProcess.ServiceName = span.Service.ServiceName
		}

		ret2 = append(ret2, model.Trace_ProcessMapping{
			ProcessID: span.ProcessID,
			Process:   modelProcess,
		})
	}

	return &model.Trace{Spans: ret, ProcessMap: ret2}, err
}

func buildTraceWhere(query *spanstore.TraceQueryParameters) *whereBuilder {
	builder := &whereBuilder{where: "", params: make([]interface{}, 0)}

	if len(query.ServiceName) > 0 {
		builder.andWhere(query.ServiceName, "service.service_name = ?")
	}
	if len(query.OperationName) > 0 {
		builder.andWhere(query.OperationName, "operation.operation_name = ?")
	}
	if query.StartTimeMin.After(time.Time{}) {
		builder.andWhere(query.StartTimeMin, "start_time >= ?")
	}
	if query.StartTimeMax.After(time.Time{}) {
		builder.andWhere(query.StartTimeMax, "start_time < ?")
	}
	if query.DurationMin > 0*time.Second {
		builder.andWhere(query.DurationMin, "duration < ?")
	}
	if query.DurationMax > 0*time.Second {
		builder.andWhere(query.DurationMax, "duration > ?")
	}

	//TODO Tags map[]string

	return builder
}

// FindTraces retrieve traces that match the traceQuery
func (r *Reader) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	r.logger.Debug("FindTraces called")

	traceIDs, err := r.FindTraceIDs(ctx, query)
	ret := make([]*model.Trace, 0, len(traceIDs))
	if err != nil {
		return ret, err
	}
	grouping := make(map[model.TraceID]*model.Trace)
	//idsLow := make([]uint64, 0, len(traceIDs))
	for _, traceID := range traceIDs {
		//idsLow = append(idsLow, traceID.Low)
		var spans []Span
		err = r.db.Model(&spans).Where("trace_id_low = ?", traceID.Low /*TODO high*/).
			//Join("JOIN operations AS operation ON operation.id = span.operation_id").
			//Join("JOIN services AS service ON service.id = span.service_id").
			Relation("Operation").Relation("Service").Order("start_time ASC").Select()
		if err != nil {
			return ret, err
		}
		for _, span := range spans {
			modelSpan := toModelSpan(span)
			trace, found := grouping[modelSpan.TraceID]
			if !found {
				trace = &model.Trace{
					Spans:      make([]*model.Span, 0, len(spans)),
					ProcessMap: make([]model.Trace_ProcessMapping, 0, len(spans)),
				}
				grouping[modelSpan.TraceID] = trace
			}
			trace.Spans = append(trace.Spans, modelSpan)
			procMap := model.Trace_ProcessMapping{
				ProcessID: span.ProcessID,
				Process: model.Process{
					ServiceName: span.Service.ServiceName,
					Tags:        mapToModelKV(span.ProcessTags),
				},
			}
			trace.ProcessMap = append(trace.ProcessMap, procMap)
		}
	}

	for _, trace := range grouping {
		ret = append(ret, trace)
	}

	return ret, err
}

// FindTraceIDs retrieve traceIDs that match the traceQuery
func (r *Reader) FindTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) (ret []model.TraceID, err error) {
	r.logger.Debug("FindTraceIds called")

	builder := buildTraceWhere(query)

	limit := query.NumTraces
	if limit <= 0 {
		limit = 10
	}

	err = r.db.Model((*Span)(nil)).
		Join("JOIN operations AS operation ON operation.id = span.operation_id").
		Join("JOIN services AS service ON service.id = span.service_id").
		ColumnExpr("distinct trace_id_low as Low, trace_id_high as High").
		Where(builder.where, builder.params...).Limit(limit).Select(&ret)

	return ret, err
}

// GetDependencies returns all inter-service dependencies
func (r *Reader) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) (ret []model.DependencyLink, err error) {
	r.logger.Debug("Get Dependencies called")

	// SQL
	query := `SELECT
		parent.service_name AS parent,
		child.service_name AS child,
		count(*) AS call_count
	FROM span_refs as refs
	JOIN spans AS source_spans ON source_spans.span_id = refs.source_span_id
	JOIN spans AS child_spans ON child_spans.span_id = refs.child_span_id
	JOIN services AS parent ON parent.id = source_spans.service_id
	JOIN services AS child ON child.id = child_spans.service_id
	GROUP BY parent, child;`

	_, err = r.db.Query(&ret, query)

	if err != nil {
		r.logger.Error("Error in querying dependencies", err)
	}

	return ret, err
}
