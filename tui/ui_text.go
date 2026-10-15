package tui

import "github.com/link-fgfgui/mod-downloader-cli/tui/i18n"

// tuiChrome holds terminal-only chrome labels that have no GUI equivalent
// (keyboard hints, dialog verbs). It is intentionally kept separate from the
// migrated GUI dictionary so the latter stays byte-for-byte identical to the
// frontend and passes its key-parity test.
var tuiChrome = map[string][2]string{
	"confirm":         {"确认", "Confirm"},
	"cancel":          {"取消", "Cancel"},
	"close":           {"关闭", "Close"},
	"save":            {"保存", "Save"},
	"back":            {"返回", "Back"},
	"open":            {"打开", "Open"},
	"create":          {"新建", "Create"},
	"rename":          {"重命名", "Rename"},
	"delete":          {"删除", "Delete"},
	"reset":           {"恢复默认", "Reset default"},
	"refresh":         {"刷新", "Refresh"},
	"export":          {"导出", "Export"},
	"copy":            {"复制", "Copy"},
	"navigate":        {"选择", "Select"},
	"deselect":        {"取消选择", "Deselect"},
	"selectAll":       {"全选", "Select all"},
	"enter":           {"回车确认", "Enter"},
	"esc":             {"退出/返回", "Esc"},
	"space":           {"空格多选", "Space"},
	"keys":            {"↑↓ 移动 · ←→ 切换 · Enter 确认 · Esc 关闭", "↑↓ move · ←→ switch · Enter confirm · Esc close"},
	"noInstance":      {"未选择实例", "No instance"},
	"loading":         {"加载中…", "Loading…"},
	"none":            {"无", "None"},
	"unknown":         {"未知", "Unknown"},
	"yes":             {"是", "Yes"},
	"no":              {"否", "No"},
	"apply":           {"应用", "Apply"},
	"continue":        {"继续", "Continue"},
	"skip":            {"跳过", "Skip"},
	"sort":            {"排序", "Sort"},
	"pin":             {"固定", "Pin"},
	"unpin":           {"取消固定", "Unpin"},
	"toggle":          {"切换", "Toggle"},
	"selectDirectory": {"选择文件夹", "Choose folder"},
	"selectFile":      {"选择保存路径", "Choose save path"},
	"typePath":        {"输入路径（支持 Tab 补全）", "Type a path (Tab to complete)"},
	"results":         {"结果", "Results"},
	"selected":        {"已选", "selected"},
	"queueRunning":    {"运行中", "running"},
	"queuePending":    {"等待中", "pending"},
	"reminders":       {"提醒", "reminders"},
	"filter":          {"过滤", "Filter"},
	"instance":        {"实例", "Instance"},
	"target":          {"目标", "Target"},
	"minecraftDir":    {".minecraft 目录", ".minecraft directory"},
	"connector":       {"Connector 视图", "Connector view"},
	"simpleMode":      {"精简模式", "Simple mode"},
	"clearAll":        {"清空全部", "Clear all"},
	"moveUp":          {"上移", "Move up"},
	"moveDown":        {"下移", "Move down"},
	"newList":         {"新建收藏夹", "New list"},
}

// uiText returns the localized chrome label for key.
func uiText(key string) string {
	entry, ok := tuiChrome[key]
	if !ok {
		return key
	}
	if i18n.Current() == i18n.LanguageChinese {
		return entry[0]
	}
	return entry[1]
}
