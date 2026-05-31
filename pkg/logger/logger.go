package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Init 初始化 Zap logger；format="json" 适合生产日志收集，format="console" 适合本地开发阅读
func Init(level, format string) (*zap.Logger, error) {
	// level 解析失败时回退到 Info，防止配置填写错误导致启动崩溃
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		zapLevel = zapcore.InfoLevel
	}

	// 配置编码器格式：定义日志各字段的 key 名、时间格式、级别格式等
	encoderCfg := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// 按 format 选择编码器：json 适合日志收集系统解析，console 适合本地肉眼阅读
	var encoder zapcore.Encoder
	if format == "json" {
		encoder = zapcore.NewJSONEncoder(encoderCfg)
	} else {
		// console 模式用彩色级别，方便本地肉眼区分 WARN / ERROR
		encoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderCfg)
	}

	// 构建 Zap core：将编码后的日志写到 stdout，并按 zapLevel 过滤低级别日志
	core := zapcore.NewCore(
		encoder,
		zapcore.AddSync(os.Stdout),
		zapLevel,
	)

	// 创建 logger：附加调用者信息（文件:行号），Error 及以上级别自动附加堆栈
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	return logger, nil
}
