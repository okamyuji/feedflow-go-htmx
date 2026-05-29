// Package obs ログ付きのリソース解放と書き込みのヘルパを提供します。
// エラーを握り潰さずloggerに記録するための共通ユーティリティです。
package obs

import (
	"fmt"
	"io"
	"log/slog"
)

// CloseAndLog cを閉じ、閉じる際のエラーがあればloggerにwarnとして記録します。
// defer obs.CloseAndLog(logger, f, "closing feeds file")の形で使い、deferでのClose漏れと
// エラー握り潰しを同時に避けます。loggerがnilの場合はslog.Defaultを用います。
func CloseAndLog(logger *slog.Logger, c io.Closer, context string) {
	if c == nil {
		return
	}
	if err := c.Close(); err != nil {
		if logger == nil {
			logger = slog.Default()
		}
		logger.Warn(context, slog.String("error", err.Error()))
	}
}

// WriteAndLog wにpを書き込み、書き込みのエラーがあればloggerにerrorとして記録して
// 文脈付きでラップしたエラーを返します。書き込んだバイト数も返します。
// loggerがnilの場合はslog.Defaultを用います。
func WriteAndLog(logger *slog.Logger, w io.Writer, p []byte, context string) (int, error) {
	n, err := w.Write(p)
	if err != nil {
		if logger == nil {
			logger = slog.Default()
		}
		logger.Error(context, slog.String("error", err.Error()))
		return n, fmt.Errorf("%s: %w", context, err)
	}
	return n, nil
}
