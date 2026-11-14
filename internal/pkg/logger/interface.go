package logger

// Logger 是服务层统一使用的格式化日志接口，参数语义与 fmt.Printf 一致。
//
// 该接口由 *zap.SugaredLogger 直接实现（由 app 注入 Logger.Sugar()），
// 不额外套包装层，因此日志中的 caller 始终指向实际写日志的位置。
type Logger interface {
	Debugf(template string, args ...any)
	Infof(template string, args ...any)
	Warnf(template string, args ...any)
	Errorf(template string, args ...any)
}
