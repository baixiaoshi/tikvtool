package ui

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/baixiaoshi/tikvtool/dao"
	"github.com/baixiaoshi/tikvtool/utils"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type KeyValue struct {
	Key   string
	Value string
}

type viewMode int

const (
	modeMain viewMode = iota
	modeSearch
	modeDetail
	modeEdit
	modeAdd
)

type model struct {
	input        string
	cursor       int
	results      []KeyValue
	selectedItem int
	searching    bool
	kvClient     *dao.RawKv
	ctx          context.Context

	// 新增字段
	mode         viewMode
	detailValue  string
	detailKey    string
	resultOffset int // 结果列表滚动偏移

	// 编辑相关字段
	editValue         string
	editCursor        int
	editLines         []string
	editLineNum       int
	waitingForSecondD bool         // Vi风格dd删除的状态
	insertMode        bool         // Vi风格：true=插入模式，false=命令模式
	commandMode       bool         // Vi风格命令行模式（:w, :x等）
	commandInput      string       // 命令行输入内容
	statusMessage     string       // 状态消息（用于显示保存状态等）
	detailCommandMode bool         // 详情模式是否为命令模式
	detailCursorLine  int          // 详情模式光标行号
	detailCursorCol   int          // 详情模式光标列号
	detailLines       []string     // 详情模式的文本行
	valueFormat       utils.Format // 当前值的格式

	// 添加模式相关字段
	addKey    string // 新增模式的 key 输入
	addValue  string // 新增模式的 value 输入
	addStep   int    // 添加步骤：0=输入key, 1=输入value
	addCursor int    // 添加模式的光标位置

	// 命令模式
	commandPrefix    string    // 当前输入的命令前缀
	isInCommand      bool      // 是否正在输入命令
	commandList      []Command // 可用命令列表
	filteredCommands []Command // 过滤后的命令列表
	selectedCommand  int       // 选中的命令索引
	commandOffset    int       // 命令列表滚动偏移
}

type searchResultMsg struct {
	results []KeyValue
	err     error
}

type deleteSuccessMsg struct {
	key string
}

type saveSuccessMsg struct {
	key          string
	value        string
	exitToDetail bool // 是否退出到详细视图
}

type addSuccessMsg struct {
	key   string
	value string
}

type Command struct {
	Name        string
	Description string
}

type saveErrorMsg struct {
	key string
	err error
}

func InitialModel(ctx context.Context, kvClient *dao.RawKv) model {
	// 初始化日志文件
	logFile, err := os.OpenFile("/tmp/test.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		log.SetOutput(logFile)
		log.Println("InitialModel: detailCommandMode set to true")
	}

	return model{
		input:             "",
		cursor:            0,
		results:           []KeyValue{},
		selectedItem:      0,
		searching:         false,
		kvClient:          kvClient,
		ctx:               ctx,
		mode:              modeMain,
		resultOffset:      0,
		detailCommandMode: true, // 默认详情模式为命令模式
		isInCommand:       true, // Main模式默认是命令模式
		commandList: []Command{
			{Name: "/search", Description: "Search keys by prefix"},
			{Name: "/add", Description: "Add new key-value pair"},
		},
		filteredCommands: []Command{
			{Name: "/search", Description: "Search keys by prefix"},
			{Name: "/add", Description: "Add new key-value pair"},
		},
		selectedCommand: 0,
		commandOffset:   0,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.mode {
		case modeMain:
			return m.updateMain(msg)
		case modeSearch:
			return m.updateSearch(msg)
		case modeDetail:
			return m.updateDetail(msg)
		case modeEdit:
			return m.updateEdit(msg)
		case modeAdd:
			return m.updateAdd(msg)
		}

	case searchResultMsg:
		m.searching = false
		if msg.err == nil {
			m.results = msg.results
			m.selectedItem = 0
			m.resultOffset = 0
		}

	case deleteSuccessMsg:
		// 删除成功，返回搜索视图并刷新结果
		m.mode = modeSearch
		return m, m.searchCmd()

	case saveSuccessMsg:
		// 保存成功，更新详细视图的内容
		m.detailValue = m.formatJSON(msg.value)
		m.statusMessage = "Saved successfully!"
		if msg.exitToDetail {
			// 如果是 :x 或 :wq 命令，退出到详细视图
			m.mode = modeDetail
			m.detailCommandMode = true // 保持命令模式
		}
		// 如果是 :w 命令，保持在编辑模式
		return m, nil

	case saveErrorMsg:
		// 保存失败，显示错误信息但保持在编辑模式
		m.statusMessage = fmt.Sprintf("Save failed: %v", msg.err)
		return m, nil

	case addSuccessMsg:
		// 添加成功，返回搜索模式并刷新结果
		m.mode = modeSearch
		m.addKey = ""
		m.addValue = ""
		m.addStep = 0
		m.addCursor = 0
		m.statusMessage = fmt.Sprintf("Added key '%s' successfully!", msg.key)
		return m, m.searchCmd()

	}

	return m, nil
}

func (m model) updateMain(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit

	case tea.KeyEsc:
		return m, tea.Quit

	case tea.KeyEnter:
		// 执行选中的命令
		return m.executeSelectedCommand()

	case tea.KeyUp:
		// 命令列表导航
		if m.selectedCommand > 0 {
			m.selectedCommand--
			if m.selectedCommand < m.commandOffset {
				m.commandOffset = m.selectedCommand
			}
		}

	case tea.KeyDown:
		// 命令列表导航
		if m.selectedCommand < len(m.filteredCommands)-1 {
			m.selectedCommand++
			if m.selectedCommand >= m.commandOffset+10 {
				m.commandOffset = m.selectedCommand - 9
			}
		}

	case tea.KeyRunes:
		// 输入来过滤命令
		if string(msg.Runes) == "/" && len(m.commandPrefix) == 0 {
			m.commandPrefix = "/"
			m.filterCommands()
			m.selectedCommand = 0
			m.commandOffset = 0
		} else if len(msg.String()) == 1 {
			m.commandPrefix += msg.String()
			m.filterCommands()
			m.selectedCommand = 0
			m.commandOffset = 0
		}

	case tea.KeyBackspace:
		// 删除字符
		if len(m.commandPrefix) > 0 {
			m.commandPrefix = m.commandPrefix[:len(m.commandPrefix)-1]
			m.filterCommands()
			m.selectedCommand = 0
			m.commandOffset = 0
		}
	}

	return m, nil
}

func (m model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit

	case tea.KeyEsc:
		// 返回Main模式
		m.mode = modeMain
		m.isInCommand = true
		m.commandPrefix = ""
		m.input = ""
		m.cursor = 0
		return m, nil

	case tea.KeyEnter:
		if len(m.results) > 0 && m.selectedItem < len(m.results) {
			// 进入详细视图，默认为命令模式
			log.Printf("Enter pressed: setting detailCommandMode to true, current value: %v", m.detailCommandMode)
			m.mode = modeDetail
			m.detailKey = m.results[m.selectedItem].Key
			m.detailValue = m.formatValue(m.results[m.selectedItem].Value)
			m.detailCommandMode = true                         // 默认进入命令模式
			m.detailLines = strings.Split(m.detailValue, "\n") // 分割文本行
			m.detailCursorLine = 0                             // 光标在第一行
			m.detailCursorCol = 0                              // 光标在第一列
			m.waitingForSecondD = false
			log.Printf("After setting: detailCommandMode = %v, lines = %d", m.detailCommandMode, len(m.detailLines))
			return m, nil
		}

	case tea.KeyUp:
		if m.selectedItem > 0 {
			m.selectedItem--
			// 自动滚动
			if m.selectedItem < m.resultOffset {
				m.resultOffset = m.selectedItem
			}
		}

	case tea.KeyDown:
		if m.selectedItem < len(m.results)-1 {
			m.selectedItem++
			// 自动滚动，假设显示10行
			if m.selectedItem >= m.resultOffset+10 {
				m.resultOffset = m.selectedItem - 9
			}
		}

	case tea.KeyLeft:
		if m.cursor > 0 {
			m.cursor--
		}

	case tea.KeyRight:
		if m.cursor < len(m.input) {
			m.cursor++
		}

	case tea.KeyBackspace:
		if m.cursor > 0 && len(m.input) > 0 {
			m.input = m.input[:m.cursor-1] + m.input[m.cursor:]
			m.cursor--
			return m, m.searchCmd()
		}

	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "d":
			// Vi风格：处理dd删除选中的key
			if m.waitingForSecondD {
				// 第二个d，执行删除选中的key
				m.waitingForSecondD = false
				if len(m.results) > 0 && m.selectedItem < len(m.results) {
					return m, m.deleteSelectedKeyCmd()
				}
				return m, nil
			} else {
				// 第一个d，等待第二个d
				m.waitingForSecondD = true
				return m, nil
			}
		default:
			// 重置等待状态并处理普通字符输入
			m.waitingForSecondD = false
			if len(msg.String()) == 1 {
				m.input = m.input[:m.cursor] + msg.String() + m.input[m.cursor:]
				m.cursor++
				return m, m.searchCmd()
			}
		}

	default:
		// 重置等待状态
		m.waitingForSecondD = false
	}

	return m, nil
}

func (m model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// 返回搜索视图
		m.mode = modeSearch
		return m, nil

	case tea.KeyCtrlC:
		return m, tea.Quit

	case tea.KeyRunes:
		if m.detailCommandMode {
			// 命令模式下的按键处理
			log.Printf("In COMMAND mode, processing key: %s", string(msg.Runes))
			switch string(msg.Runes) {
			case "i", "I":
				// Vi风格：i进入编辑模式（命令模式）
				m.mode = modeEdit
				m.editValue = m.detailValue
				m.editLines = strings.Split(m.editValue, "\n")
				m.editLineNum = 0
				m.editCursor = 0
				m.insertMode = false // 开始是命令模式
				return m, nil
			case "d":
				// Vi风格：处理dd删除当前key
				if m.waitingForSecondD {
					// 第二个d，执行删除当前key
					m.waitingForSecondD = false
					return m, m.deleteCurrentKeyCmd()
				} else {
					// 第一个d，等待第二个d
					m.waitingForSecondD = true
					return m, nil
				}
			case "v":
				// 切换到普通浏览模式
				m.detailCommandMode = false
				m.waitingForSecondD = false
				return m, nil
			case "j":
				// 向下移动光标
				log.Printf("COMMAND mode: j key pressed (move cursor down)")
				if m.detailCursorLine < len(m.detailLines)-1 {
					m.detailCursorLine++
					// 确保光标列不超出当前行长度
					if m.detailCursorCol > len(m.detailLines[m.detailCursorLine]) {
						m.detailCursorCol = len(m.detailLines[m.detailCursorLine])
					}
				}
				m.waitingForSecondD = false
				return m, nil
			case "k":
				// 向上移动光标
				log.Printf("COMMAND mode: k key pressed (move cursor up)")
				if m.detailCursorLine > 0 {
					m.detailCursorLine--
					// 确保光标列不超出当前行长度
					if m.detailCursorCol > len(m.detailLines[m.detailCursorLine]) {
						m.detailCursorCol = len(m.detailLines[m.detailCursorLine])
					}
				}
				m.waitingForSecondD = false
				return m, nil
			case "h":
				// 向左移动光标
				log.Printf("COMMAND mode: h key pressed (move cursor left)")
				if m.detailCursorCol > 0 {
					m.detailCursorCol--
				} else if m.detailCursorLine > 0 {
					// 到上一行末尾
					m.detailCursorLine--
					m.detailCursorCol = len(m.detailLines[m.detailCursorLine])
				}
				m.waitingForSecondD = false
				return m, nil
			case "l":
				// 向右移动光标
				log.Printf("COMMAND mode: l key pressed (move cursor right)")
				if m.detailCursorLine < len(m.detailLines) && m.detailCursorCol < len(m.detailLines[m.detailCursorLine]) {
					m.detailCursorCol++
				} else if m.detailCursorLine < len(m.detailLines)-1 {
					// 到下一行开头
					m.detailCursorLine++
					m.detailCursorCol = 0
				}
				m.waitingForSecondD = false
				return m, nil
			default:
				// 重置等待状态
				m.waitingForSecondD = false
			}
		} else {
			// 普通浏览模式下的按键处理
			switch string(msg.Runes) {
			case "i", "I":
				// Vi风格：i进入编辑模式（命令模式）
				m.mode = modeEdit
				m.editValue = m.detailValue
				m.editLines = strings.Split(m.editValue, "\n")
				m.editLineNum = 0
				m.editCursor = 0
				m.insertMode = false // 开始是命令模式
				return m, nil
			case "c":
				// 切换到命令模式
				m.detailCommandMode = true
				m.waitingForSecondD = false
				return m, nil
			}
		}

	}

	return m, nil
}

// formatValue 格式化值并返回格式信息
func (m *model) formatValue(value string) string {
	if len(value) == 0 {
		m.valueFormat = utils.FormatPlainText
		return "<empty>"
	}

	formatted, format := utils.FormatContent(value)
	m.valueFormat = format
	return formatted
}

// formatJSON 兼容性方法，使用新的 formatValue
func (m model) formatJSON(value string) string {
	formatted, _ := utils.FormatContent(value)
	return formatted
}

// updateEdit 处理编辑模式的按键（Vi风格）
func (m model) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.commandMode {
		// 命令行模式（:w, :x等）
		return m.updateEditCommandLine(msg)
	} else if !m.insertMode {
		// 命令模式
		return m.updateEditCommand(msg)
	} else {
		// 插入模式
		return m.updateEditInsert(msg)
	}
}

// updateEditCommand Vi命令模式
func (m model) updateEditCommand(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// 退出编辑，返回详细视图
		m.mode = modeDetail
		m.detailCommandMode = true // 保持命令模式
		return m, nil

	case tea.KeyCtrlC:
		return m, tea.Quit

	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "i":
			// 进入插入模式
			m.insertMode = true
			return m, nil
		case "I":
			// 在行首进入插入模式
			m.editCursor = 0
			m.insertMode = true
			return m, nil
		case "a":
			// 在光标后进入插入模式
			if m.editLineNum < len(m.editLines) && m.editCursor < len(m.editLines[m.editLineNum]) {
				m.editCursor++
			}
			m.insertMode = true
			return m, nil
		case "A":
			// 在行尾进入插入模式
			if m.editLineNum < len(m.editLines) {
				m.editCursor = len(m.editLines[m.editLineNum])
			}
			m.insertMode = true
			return m, nil
		case "o":
			// 在当前行下方新建一行并进入插入模式
			newLines := make([]string, len(m.editLines)+1)
			copy(newLines, m.editLines[:m.editLineNum+1])
			newLines[m.editLineNum+1] = ""
			copy(newLines[m.editLineNum+2:], m.editLines[m.editLineNum+1:])
			m.editLines = newLines
			m.editLineNum++
			m.editCursor = 0
			m.insertMode = true
			return m, nil
		case "O":
			// 在当前行上方新建一行并进入插入模式
			newLines := make([]string, len(m.editLines)+1)
			copy(newLines, m.editLines[:m.editLineNum])
			newLines[m.editLineNum] = ""
			copy(newLines[m.editLineNum+1:], m.editLines[m.editLineNum:])
			m.editLines = newLines
			m.editCursor = 0
			m.insertMode = true
			return m, nil
		case "h":
			// 左移
			if m.editCursor > 0 {
				m.editCursor--
			}
			return m, nil
		case "l":
			// 右移
			if m.editLineNum < len(m.editLines) && m.editCursor < len(m.editLines[m.editLineNum]) {
				m.editCursor++
			}
			return m, nil
		case "j":
			// 下移
			if m.editLineNum < len(m.editLines)-1 {
				m.editLineNum++
				if m.editLineNum < len(m.editLines) && m.editCursor > len(m.editLines[m.editLineNum]) {
					m.editCursor = len(m.editLines[m.editLineNum])
				}
			}
			return m, nil
		case "k":
			// 上移
			if m.editLineNum > 0 {
				m.editLineNum--
				if m.editLineNum < len(m.editLines) && m.editCursor > len(m.editLines[m.editLineNum]) {
					m.editCursor = len(m.editLines[m.editLineNum])
				}
			}
			return m, nil
		case ":":
			// 进入命令行模式
			m.commandMode = true
			m.commandInput = ":"
			return m, nil
		case "d":
			// Vi风格：处理dd删除行
			if m.waitingForSecondD {
				// 第二个d，执行删除行
				m.waitingForSecondD = false
				if len(m.editLines) > 1 {
					m.editLines = append(m.editLines[:m.editLineNum], m.editLines[m.editLineNum+1:]...)
					if m.editLineNum >= len(m.editLines) {
						m.editLineNum = len(m.editLines) - 1
					}
				} else if len(m.editLines) == 1 {
					m.editLines[0] = ""
				}
				m.editCursor = 0
				return m, nil
			} else {
				// 第一个d，等待第二个d
				m.waitingForSecondD = true
				return m, nil
			}
		}

	default:
		// 重置等待状态（如果按了其他键）
		m.waitingForSecondD = false
	}

	// 处理特殊按键组合保存 (Ctrl+S 或 ZZ)
	if msg.Type == tea.KeyCtrlS {
		newValue := strings.Join(m.editLines, "\n")
		return m, m.saveKeyCmd(newValue, false)
	}

	return m, nil
}

// updateEditCommandLine Vi命令行模式（:w, :x等）
func (m model) updateEditCommandLine(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// 退出命令行模式
		m.commandMode = false
		m.commandInput = ""
		return m, nil

	case tea.KeyEnter:
		// 执行命令
		cmd := m.commandInput
		m.commandMode = false
		m.commandInput = ""
		m.insertMode = false // 重置插入模式
		m.statusMessage = "" // 清除状态消息

		switch cmd {
		case ":w":
			// 保存文件，保持在编辑模式
			newValue := strings.Join(m.editLines, "\n")

			return m, m.saveKeyCmd(newValue, false)
		case ":x", ":wq":
			// 保存并退出
			newValue := strings.Join(m.editLines, "\n")
			// 先保存，然后在保存成功后会自动返回详细视图
			return m, m.saveKeyCmd(newValue, true)
		case ":q":
			// 退出（不保存）
			m.mode = modeDetail
			m.detailCommandMode = true // 保持命令模式
			return m, nil
		case ":q!":
			// 强制退出（不保存）
			m.mode = modeDetail
			m.detailCommandMode = true // 保持命令模式
			return m, nil
		}
		return m, nil

	case tea.KeyBackspace:
		// 删除命令字符
		if len(m.commandInput) > 1 { // 保留 ":"
			m.commandInput = m.commandInput[:len(m.commandInput)-1]
		}
		return m, nil

	default:
		// 添加字符到命令
		if len(msg.String()) == 1 {
			m.commandInput += msg.String()
		}
		return m, nil
	}
}

// updateEditInsert Vi插入模式
func (m model) updateEditInsert(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// 退出插入模式，回到命令模式
		m.insertMode = false
		return m, nil

	case tea.KeyCtrlC:
		return m, tea.Quit

	case tea.KeyUp:
		// 上移
		if m.editLineNum > 0 {
			m.editLineNum--
			if m.editLineNum < len(m.editLines) && m.editCursor > len(m.editLines[m.editLineNum]) {
				m.editCursor = len(m.editLines[m.editLineNum])
			}
		}
		return m, nil

	case tea.KeyDown:
		// 下移
		if m.editLineNum < len(m.editLines)-1 {
			m.editLineNum++
			if m.editLineNum < len(m.editLines) && m.editCursor > len(m.editLines[m.editLineNum]) {
				m.editCursor = len(m.editLines[m.editLineNum])
			}
		}
		return m, nil

	case tea.KeyLeft:
		// 左移
		if m.editCursor > 0 {
			m.editCursor--
		} else if m.editLineNum > 0 {
			// 到上一行末尾
			m.editLineNum--
			m.editCursor = len(m.editLines[m.editLineNum])
		}
		return m, nil

	case tea.KeyRight:
		// 右移
		if m.editLineNum < len(m.editLines) && m.editCursor < len(m.editLines[m.editLineNum]) {
			m.editCursor++
		} else if m.editLineNum < len(m.editLines)-1 {
			// 到下一行开头
			m.editLineNum++
			m.editCursor = 0
		}
		return m, nil

	case tea.KeyBackspace:
		if m.editLineNum < len(m.editLines) {
			if m.editCursor > 0 {
				// 删除当前行的字符
				line := m.editLines[m.editLineNum]
				m.editLines[m.editLineNum] = line[:m.editCursor-1] + line[m.editCursor:]
				m.editCursor--
			} else if m.editLineNum > 0 {
				// 合并到上一行
				prevLine := m.editLines[m.editLineNum-1]
				currentLine := m.editLines[m.editLineNum]
				m.editLines[m.editLineNum-1] = prevLine + currentLine
				m.editLines = append(m.editLines[:m.editLineNum], m.editLines[m.editLineNum+1:]...)
				m.editLineNum--
				m.editCursor = len(prevLine)
			}
		}

	case tea.KeyEnter:
		// 换行
		if m.editLineNum < len(m.editLines) {
			line := m.editLines[m.editLineNum]
			beforeCursor := line[:m.editCursor]
			afterCursor := line[m.editCursor:]
			m.editLines[m.editLineNum] = beforeCursor
			newLines := make([]string, len(m.editLines)+1)
			copy(newLines, m.editLines[:m.editLineNum+1])
			newLines[m.editLineNum+1] = afterCursor
			copy(newLines[m.editLineNum+2:], m.editLines[m.editLineNum+1:])
			m.editLines = newLines
			m.editLineNum++
			m.editCursor = 0
		}

	default:
		// 输入普通字符
		if len(msg.String()) == 1 && m.editLineNum < len(m.editLines) {
			line := m.editLines[m.editLineNum]
			m.editLines[m.editLineNum] = line[:m.editCursor] + msg.String() + line[m.editCursor:]
			m.editCursor++
		}
	}

	return m, nil
}

// deleteCurrentKeyCmd 删除详情模式中当前的key
func (m model) deleteCurrentKeyCmd() tea.Cmd {
	key := m.detailKey
	return func() tea.Msg {
		err := m.kvClient.Delete(m.ctx, []byte(key))
		if err != nil {
			return searchResultMsg{results: nil, err: err}
		}
		// 删除成功，返回搜索视图并刷新结果
		return deleteSuccessMsg{key: key}
	}
}

// deleteSelectedKeyCmd 删除搜索模式中选中的key
func (m model) deleteSelectedKeyCmd() tea.Cmd {
	if len(m.results) == 0 || m.selectedItem >= len(m.results) {
		return nil
	}
	key := m.results[m.selectedItem].Key
	return func() tea.Msg {
		err := m.kvClient.Delete(m.ctx, []byte(key))
		if err != nil {
			return searchResultMsg{results: nil, err: err}
		}
		// 删除成功，刷新搜索结果
		return deleteSuccessMsg{key: key}
	}
}

// saveKeyCmd 保存编辑后的value
func (m model) saveKeyCmd(newValue string, exitToDetail bool) tea.Cmd {
	key := m.detailKey
	return func() tea.Msg {

		err := m.kvClient.Put(m.ctx, []byte(key), []byte(newValue))

		if err != nil {
			return saveErrorMsg{key: key, err: err}
		}
		return saveSuccessMsg{key: key, value: newValue, exitToDetail: exitToDetail}
	}
}

func (m model) searchCmd() tea.Cmd {
	if len(m.input) == 0 {
		// 如果没有输入，显示所有key（不限制前缀）
		return func() tea.Msg {
			keys, vals, err := m.kvClient.ScanAllKeys(m.ctx, 50)
			if err != nil {
				return searchResultMsg{results: nil, err: err}
			}

			results := make([]KeyValue, len(keys))
			for i, key := range keys {
				// 存储完整的值，用于详细视图
				val := string(vals[i])
				if len(val) == 0 {
					val = "" // 空值直接设为空字符串
				}

				keyStr := string(key)
				if len(keyStr) == 0 {
					keyStr = "<empty key>"
				}
				results[i] = KeyValue{
					Key:   keyStr,
					Value: val,
				}
			}
			return searchResultMsg{results: results, err: nil}
		}
	}

	m.searching = true
	input := m.input

	return func() tea.Msg {
		// 直接使用用户输入的内容作为前缀，不添加任何前缀
		keys, vals, err := m.kvClient.ScanWithRealPrefix(m.ctx, []byte(input), 50)

		if err != nil {
			return searchResultMsg{results: nil, err: err}
		}

		results := make([]KeyValue, len(keys))
		for i, key := range keys {
			// 存储完整的值，用于详细视图
			val := string(vals[i])
			if len(val) == 0 {
				val = "" // 空值直接设为空字符串
			}

			keyStr := string(key)
			if len(keyStr) == 0 {
				keyStr = "<empty key>"
			}
			results[i] = KeyValue{
				Key:   keyStr,
				Value: val,
			}
		}

		return searchResultMsg{results: results, err: nil}
	}
}

// updateAdd 处理添加模式的按键
func (m model) updateAdd(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// 返回Main模式
		m.mode = modeMain
		m.isInCommand = true
		m.commandPrefix = ""
		m.addKey = ""
		m.addValue = ""
		m.addStep = 0
		m.addCursor = 0
		return m, nil

	case tea.KeyCtrlC:
		return m, tea.Quit

	case tea.KeyTab:
		// Tab 键切换输入框焦点
		if m.addStep == 0 && len(strings.TrimSpace(m.addKey)) > 0 {
			m.addStep = 1
			m.addCursor = len(m.addValue)
		} else if m.addStep == 1 {
			m.addStep = 0
			m.addCursor = len(m.addKey)
		}
		return m, nil

	case tea.KeyEnter:
		if m.addStep == 0 {
			// 在 key 输入模式下，按 Enter 切换到 value 输入
			if len(strings.TrimSpace(m.addKey)) > 0 {
				m.addStep = 1
				m.addCursor = len(m.addValue)
			}
		} else {
			// 在 value 输入模式下，按 Enter 换行
			currentInput := m.getCurrentInput()
			newInput := currentInput[:m.addCursor] + "\n" + currentInput[m.addCursor:]
			m.setCurrentInput(newInput)
			m.addCursor++
		}
		return m, nil

	case tea.KeyCtrlS:
		// Ctrl+S 保存键值对
		if len(strings.TrimSpace(m.addKey)) > 0 {
			return m, m.addKeyValueCmd()
		}
		return m, nil

	case tea.KeyLeft:
		if m.addCursor > 0 {
			m.addCursor--
		}
		return m, nil

	case tea.KeyRight:
		currentInput := m.getCurrentInput()
		if m.addCursor < len(currentInput) {
			m.addCursor++
		}
		return m, nil

	case tea.KeyBackspace:
		if m.addCursor > 0 {
			currentInput := m.getCurrentInput()
			newInput := currentInput[:m.addCursor-1] + currentInput[m.addCursor:]
			m.setCurrentInput(newInput)
			m.addCursor--
		}
		return m, nil

	default:
		// 输入普通字符
		if len(msg.String()) == 1 {
			currentInput := m.getCurrentInput()
			newInput := currentInput[:m.addCursor] + msg.String() + currentInput[m.addCursor:]
			m.setCurrentInput(newInput)
			m.addCursor++
		}
		return m, nil
	}
}

// getCurrentInput 获取当前输入
func (m model) getCurrentInput() string {
	if m.addStep == 0 {
		return m.addKey
	}
	return m.addValue
}

// setCurrentInput 设置当前输入
func (m *model) setCurrentInput(input string) {
	if m.addStep == 0 {
		m.addKey = input
	} else {
		m.addValue = input
	}
}

// addKeyValueCmd 添加键值对
func (m model) addKeyValueCmd() tea.Cmd {
	key := strings.TrimSpace(m.addKey)
	value := m.addValue
	return func() tea.Msg {
		err := m.kvClient.Put(m.ctx, []byte(key), []byte(value))
		if err != nil {
			return saveErrorMsg{key: key, err: err}
		}
		return addSuccessMsg{key: key, value: value}
	}
}

// executeSelectedCommand 执行选中的命令
func (m model) executeSelectedCommand() (tea.Model, tea.Cmd) {
	if m.selectedCommand >= len(m.filteredCommands) {
		return m, nil
	}

	selectedCmd := m.filteredCommands[m.selectedCommand]
	m.isInCommand = false
	m.commandPrefix = ""

	switch selectedCmd.Name {
	case "/search":
		// 切换到搜索模式
		m.mode = modeSearch
		m.input = ""
		m.cursor = 0
		m.statusMessage = ""
		return m, nil
	case "/add":
		// 切换到添加模式
		m.mode = modeAdd
		m.addStep = 0
		m.addKey = ""
		m.addValue = ""
		m.addCursor = 0
		m.statusMessage = ""
		return m, nil
	default:
		return m, nil
	}
}

// filterCommands 根据输入过滤命令列表
func (m *model) filterCommands() {
	m.filteredCommands = []Command{}
	for _, cmd := range m.commandList {
		if strings.HasPrefix(cmd.Name, m.commandPrefix) {
			m.filteredCommands = append(m.filteredCommands, cmd)
		}
	}
}

func (m model) View() string {
	switch m.mode {
	case modeMain:
		return m.viewMain()
	case modeSearch:
		return m.viewSearch()
	case modeDetail:
		return m.viewDetail()
	case modeEdit:
		return m.viewEdit()
	case modeAdd:
		return m.viewAdd()
	default:
		return m.viewMain()
	}
}

func (m model) viewMain() string {
	var s strings.Builder

	// 标题
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		PaddingBottom(1).
		Render("🔍 TiKV Key Explorer")
	s.WriteString(title + "\n")

	// 输入框区域
	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#874BFD")).
		Padding(0, 1).
		Height(1).
		Width(100)

	// 显示命令输入
	prompt := "> "
	input := m.commandPrefix + "|"
	inputBox := inputStyle.Render(prompt + input)
	s.WriteString(inputBox + "\n\n")

	// 显示命令列表
	m.renderCommandList(&s)

	// 帮助信息
	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#626262")).
		MarginTop(1)
	helpText := "• ↑/↓ select command • Enter to execute • Type to filter • Esc quit"
	s.WriteString("\n" + help.Render(helpText))

	// 模式指示器
	modeIndicator := "---Main---"
	modeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		MarginTop(1)
	s.WriteString("\n" + modeStyle.Render(modeIndicator))

	return s.String()
}

func (m model) viewSearch() string {
	var s strings.Builder

	// 标题
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		PaddingBottom(1).
		Render("🔍 TiKV Key Explorer")
	s.WriteString(title + "\n")

	// 输入框区域（固定高度）
	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#874BFD")).
		Padding(0, 1).
		Height(1).
		Width(100)

	// 始终使用 > 提示符
	prompt := "> "
	input := m.input
	// 添加光标
	if m.cursor < len(input) {
		input = input[:m.cursor] + "|" + input[m.cursor+1:]
	} else {
		input += "|"
	}

	inputBox := inputStyle.Render(prompt + input)
	s.WriteString(inputBox + "\n\n")

	// 搜索状态或结果
	if m.searching {
		searching := lipgloss.NewStyle().
			Italic(true).
			Foreground(lipgloss.Color("#626262")).
			Render("Searching...")
		s.WriteString(searching + "\n")
	} else {
		// 结果区域（固定区域，不会把输入框挤掉）
		m.renderResults(&s)
	}

	// 帮助信息
	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#626262")).
		MarginTop(1)

	var helpText string
	if len(m.input) > 0 || len(m.results) > 0 {
		helpText = "• ↑/↓ navigate • Enter view • dd delete • Esc to main"
	} else {
		helpText = "• Start typing to search • Esc to main"
	}

	s.WriteString("\n" + help.Render(helpText))

	// 模式指示器
	modeIndicator := "---Search---"

	modeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		MarginTop(1)
	s.WriteString("\n" + modeStyle.Render(modeIndicator))

	return s.String()
}

func (m model) renderResults(s *strings.Builder) {
	if len(m.results) == 0 {
		if len(m.input) > 0 {
			noResults := lipgloss.NewStyle().
				Italic(true).
				Foreground(lipgloss.Color("#626262")).
				Render("No results found")
			s.WriteString(noResults + "\n")
		}
		return
	}

	// 结果标题
	resultsTitle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#04B575")).
		Render(fmt.Sprintf("---------------------- results (%d) ----------------------", len(m.results)))
	s.WriteString(resultsTitle + "\n")

	// 显示10行结果（带滚动）
	maxDisplay := 10
	start := m.resultOffset
	end := start + maxDisplay
	if end > len(m.results) {
		end = len(m.results)
	}

	for i := start; i < end; i++ {
		result := m.results[i]
		var style lipgloss.Style

		if i == m.selectedItem {
			// 选中项使用蓝色背景
			style = lipgloss.NewStyle().
				Background(lipgloss.Color("#3b82f6")).
				Foreground(lipgloss.Color("#ffffff")).
				Padding(0, 1)
		} else {
			// 普通项使用灰白色
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#9ca3af")).
				Padding(0, 1)
		}

		// 只显示 key，不显示 value
		keyText := result.Key
		if len(keyText) > 120 {
			keyText = keyText[:117] + "..."
		}

		line := keyText
		s.WriteString(style.Render(line) + "\n")
	}

	// 滚动指示器
	if len(m.results) > maxDisplay {
		scrollInfo := fmt.Sprintf("[%d-%d of %d]", start+1, end, len(m.results))
		scrollStyle := lipgloss.NewStyle().
			Italic(true).
			Foreground(lipgloss.Color("#6b7280"))
		s.WriteString(scrollStyle.Render(scrollInfo) + "\n")
	}
}

func (m model) viewDetail() string {
	var s strings.Builder

	// 标题
	var titleText string
	if m.detailCommandMode {
		titleText = "📝 Detail View -- NORMAL --"
	} else {
		titleText = "📝 Detail View -- VIEW --"
	}
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		PaddingBottom(1).
		Render(titleText)
	s.WriteString(title + "\n")

	// Key 显示
	keyStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#10b981")).
		PaddingBottom(1)
	s.WriteString(keyStyle.Render("Key:") + "\n")
	s.WriteString(m.detailKey + "\n\n")

	// Value 显示（显示检测到的格式）
	valueStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#10b981")).
		PaddingBottom(1)
	formatName := utils.GetFormatName(m.valueFormat)
	s.WriteString(valueStyle.Render(fmt.Sprintf("Value (%s):", formatName)) + "\n")

	// JSON 内容显示区域 - 不使用语法高亮
	jsonStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6b7280")).
		Padding(1).
		MaxWidth(120)

	// 构建带光标的JSON内容
	var jsonContent strings.Builder
	for i, line := range m.detailLines {
		if i == m.detailCursorLine && m.detailCommandMode {
			// 当前行显示光标
			if m.detailCursorCol <= len(line) {
				beforeCursor := line[:m.detailCursorCol]
				afterCursor := line[m.detailCursorCol:]
				cursorStyle := lipgloss.NewStyle().Background(lipgloss.Color("#ffffff")).Foreground(lipgloss.Color("#000000"))
				if len(afterCursor) > 0 {
					jsonContent.WriteString(beforeCursor + cursorStyle.Render(string(afterCursor[0])) + afterCursor[1:])
				} else {
					jsonContent.WriteString(beforeCursor + cursorStyle.Render(" "))
				}
			} else {
				jsonContent.WriteString(line)
			}
		} else {
			jsonContent.WriteString(line)
		}
		if i < len(m.detailLines)-1 {
			jsonContent.WriteString("\n")
		}
	}

	if len(m.detailLines) == 0 {
		jsonContent.WriteString("<empty value>")
	}

	s.WriteString(jsonStyle.Render(jsonContent.String()) + "\n\n")

	// 帮助信息
	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#626262"))

	var helpText string
	if m.detailCommandMode {
		helpText = "• Esc return • dd delete • i edit • v view mode"
	} else {
		helpText = "• Esc return • c command mode • i edit"
	}

	s.WriteString(help.Render(helpText))

	return s.String()
}

// renderCommandList 渲染命令列表
func (m model) renderCommandList(s *strings.Builder) {
	if len(m.filteredCommands) == 0 {
		// 没有匹配的命令
		noMatch := lipgloss.NewStyle().
			Italic(true).
			Foreground(lipgloss.Color("#626262")).
			Render("No matching commands")
		s.WriteString(noMatch + "\n")
		return
	}

	// 命令列表标题
	commandTitle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#04B575")).
		Render(fmt.Sprintf("Available Commands (%d)", len(m.filteredCommands)))
	s.WriteString(commandTitle + "\n")

	// 显示10行命令（带滚动）
	maxDisplay := 10
	start := m.commandOffset
	end := start + maxDisplay
	if end > len(m.filteredCommands) {
		end = len(m.filteredCommands)
	}

	for i := start; i < end; i++ {
		cmd := m.filteredCommands[i]
		var style lipgloss.Style

		if i == m.selectedCommand {
			// 选中项使用蓝色背景
			style = lipgloss.NewStyle().
				Background(lipgloss.Color("#3b82f6")).
				Foreground(lipgloss.Color("#ffffff")).
				Padding(0, 1)
		} else {
			// 普通项使用灰白色
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#9ca3af")).
				Padding(0, 1)
		}

		line := fmt.Sprintf("%-10s %s", cmd.Name, cmd.Description)
		s.WriteString(style.Render(line) + "\n")
	}

	// 滚动指示器
	if len(m.filteredCommands) > maxDisplay {
		scrollInfo := fmt.Sprintf("[%d-%d of %d]", start+1, end, len(m.filteredCommands))
		scrollStyle := lipgloss.NewStyle().
			Italic(true).
			Foreground(lipgloss.Color("#6b7280"))
		s.WriteString(scrollStyle.Render(scrollInfo) + "\n")
	}
}

// viewAdd 显示添加模式
func (m model) viewAdd() string {
	var s strings.Builder

	// 标题
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		PaddingBottom(1).
		Render("🔍 TiKV Key Explorer")
	s.WriteString(title + "\n")

	// 显示当前步骤信息
	stepInfo := ""
	if m.addStep == 0 {
		stepInfo = "Step 1/2: Enter Key"
	} else {
		stepInfo = "Step 2/2: Enter Value"
	}
	stepStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#10b981")).
		PaddingBottom(1)
	s.WriteString(stepStyle.Render(stepInfo) + "\n")

	// 输入框区域
	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#874BFD")).
		Padding(0, 1).
		Height(1).
		Width(100)

	if m.addStep == 0 {
		// 当前正在输入 key
		keyInput := m.addKey
		if m.addCursor <= len(keyInput) {
			keyInput = keyInput[:m.addCursor] + "|" + keyInput[m.addCursor:]
		}

		// 高亮当前输入框
		activeKeyStyle := inputStyle.BorderForeground(lipgloss.Color("#FF6B6B"))
		inactiveStyle := inputStyle.BorderForeground(lipgloss.Color("#666666"))

		s.WriteString("Key: \n")
		s.WriteString(activeKeyStyle.Render(keyInput) + "\n\n")
		s.WriteString("Value: \n")
		s.WriteString(inactiveStyle.Render("") + "\n\n")
	} else {
		// 正在输入 value
		inactiveStyle := inputStyle.BorderForeground(lipgloss.Color("#666666"))

		s.WriteString("Key: \n")
		s.WriteString(inactiveStyle.Render(m.addKey) + "\n\n")

		// 检测并显示格式
		_, format := utils.FormatContent(m.addValue)
		formatName := utils.GetFormatName(format)
		formatIndicator := lipgloss.NewStyle().
			Italic(true).
			Foreground(lipgloss.Color("#6b7280")).
			Render(fmt.Sprintf("Value (Detected: %s):", formatName))

		s.WriteString(formatIndicator + "\n")

		// 支持多行输入
		valueInput := m.addValue
		if m.addCursor <= len(valueInput) {
			valueInput = valueInput[:m.addCursor] + "|" + valueInput[m.addCursor:]
		}

		// 使用多行显示
		multilineStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FF6B6B")).
			Padding(1).
			Height(8).
			Width(100)
		s.WriteString(multilineStyle.Render(valueInput) + "\n\n")
	}

	// 状态消息
	if m.statusMessage != "" {
		statusStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#10b981"))
		s.WriteString(statusStyle.Render(m.statusMessage) + "\n")
	}

	// 帮助信息
	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#626262"))

	if m.addStep == 0 {
		s.WriteString(help.Render("• Tab/Enter to switch to value • Esc to cancel"))
	} else {
		s.WriteString(help.Render("• Tab to switch to key • Enter for newline • Ctrl+S to save • Esc to cancel"))
	}

	// 添加模式指示器
	modeIndicator := "---Add---"
	modeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		MarginTop(1)
	s.WriteString("\n" + modeStyle.Render(modeIndicator))

	return s.String()
}

func (m model) viewEdit() string {
	var s strings.Builder

	// 标题
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		PaddingBottom(1).
		Render("✏️  Edit Mode")
	s.WriteString(title + "\n")

	// Key 显示
	keyStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#10b981")).
		PaddingBottom(1)
	s.WriteString(keyStyle.Render("Key:") + "\n")
	s.WriteString(m.detailKey + "\n\n")

	// 编辑区域标题
	valueStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#10b981")).
		PaddingBottom(1)
	s.WriteString(valueStyle.Render("Edit Value:") + "\n")

	// 编辑内容显示
	editStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6b7280")).
		Padding(1).
		MaxWidth(120)

	// 构建编辑内容，显示光标
	var editContent strings.Builder
	for i, line := range m.editLines {
		if i == m.editLineNum {
			// 当前行，显示光标
			if m.editCursor <= len(line) {
				beforeCursor := line[:m.editCursor]
				afterCursor := line[m.editCursor:]
				cursorStyle := lipgloss.NewStyle().Background(lipgloss.Color("#ffffff")).Foreground(lipgloss.Color("#000000"))
				if len(afterCursor) > 0 {
					editContent.WriteString(beforeCursor + cursorStyle.Render(string(afterCursor[0])) + afterCursor[1:])
				} else {
					editContent.WriteString(beforeCursor + cursorStyle.Render(" "))
				}
			} else {
				editContent.WriteString(line)
			}
		} else {
			editContent.WriteString(line)
		}
		if i < len(m.editLines)-1 {
			editContent.WriteString("\n")
		}
	}

	s.WriteString(editStyle.Render(editContent.String()) + "\n\n")

	// 显示当前模式和帮助信息
	var modeText string
	if m.commandMode {
		modeText = "-- COMMAND LINE --"
	} else if m.insertMode {
		modeText = "-- INSERT --"
	} else {
		modeText = "-- COMMAND --"
	}

	modeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4"))
	s.WriteString(modeStyle.Render(modeText) + "\n")

	// 如果在命令行模式，显示命令输入
	if m.commandMode {
		commandStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#874BFD")).
			Padding(0, 1)
		s.WriteString(commandStyle.Render(m.commandInput+"_") + "\n")
	}

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#626262"))

	// 显示状态消息（如果有的话）
	if m.statusMessage != "" {
		statusStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#10b981"))
		s.WriteString(statusStyle.Render(m.statusMessage) + "\n")
	}

	if m.commandMode {
		s.WriteString(help.Render("• :w to save • :x to save and exit • :q to quit • Esc to cancel"))
	} else if m.insertMode {
		s.WriteString(help.Render("• Esc then :w to save • Esc then :x to save and exit • Ctrl+S to save"))
	} else {
		s.WriteString(help.Render("• i/a/o to insert • hjkl to move • dd to delete line • : for commands • Esc to exit"))
	}

	return s.String()
}
