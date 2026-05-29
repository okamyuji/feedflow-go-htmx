package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// filePerm永続化ファイルのパーミッションです。所有者の読み書きのみを許可します。
const filePerm = 0o600

// writeJSONAtomic vをJSONへ整形してpathにアトミックに書き込みます。
// 同一ディレクトリ内の一時ファイルへ書いてからos.Renameで置き換えるため、書き込み途中の
// クラッシュでpathが破損することを避けます。書き込み失敗時は一時ファイルを後始末します。
func writeJSONAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal json for %s: %w", path, err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()        //nolint:errcheck // 異常系の後始末のため主たるエラーを優先する
		_ = os.Remove(tmpName) //nolint:errcheck // 異常系の後始末のため主たるエラーを優先する
		return fmt.Errorf("failed to write temp file %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()        //nolint:errcheck // 異常系の後始末のため主たるエラーを優先する
		_ = os.Remove(tmpName) //nolint:errcheck // 異常系の後始末のため主たるエラーを優先する
		return fmt.Errorf("failed to sync temp file %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName) //nolint:errcheck // 異常系の後始末のため主たるエラーを優先する
		return fmt.Errorf("failed to close temp file %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, filePerm); err != nil {
		_ = os.Remove(tmpName) //nolint:errcheck // 異常系の後始末のため主たるエラーを優先する
		return fmt.Errorf("failed to chmod temp file %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName) //nolint:errcheck // 異常系の後始末のため主たるエラーを優先する
		return fmt.Errorf("failed to rename temp file to %s: %w", path, err)
	}
	return nil
}

// readJSON pathのJSONをvにデコードします。ファイルが存在しない場合はos.IsNotExistで
// 判別できるエラーをそのまま返し、呼び出し側が既定値へのフォールバックを判断できるようにします。
func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return err
		}
		return fmt.Errorf("failed to read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("failed to unmarshal %s: %w", path, err)
	}
	return nil
}
