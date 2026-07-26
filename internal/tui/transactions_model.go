package tui

import (
	"context"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matteing/monarch-cli/internal/monarch"
)

type transactionPageMsg struct {
	index int
	page  monarch.TransactionPage
	err   error
}

type transactionModel struct {
	opts     TransactionOptions
	viewport viewport.Model
	spinner  spinner.Model
	pages    map[int]monarch.TransactionPage
	page     int
	width    int
	height   int
	loading  bool
	err      error
	cancel   context.CancelFunc
	canceled bool
}

func newTransactionModel(opts TransactionOptions) transactionModel {
	indicator := spinner.New(spinner.WithSpinner(spinner.Dot))
	indicator.Style = lipgloss.NewStyle().Foreground(accentColor)
	model := transactionModel{
		opts: opts, viewport: viewport.New(), spinner: indicator,
		pages: make(map[int]monarch.TransactionPage),
		width: 100, height: 24, loading: true, cancel: func() {},
	}
	model.resize(model.width, model.height)
	return model
}

func (m transactionModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetch(0, m.opts.InitialCursor))
}

func (m transactionModel) fetch(index int, cursor string) tea.Cmd {
	ctx := m.opts.Context
	fetch := m.opts.Fetch
	return func() tea.Msg {
		page, err := fetch(ctx, cursor)
		return transactionPageMsg{index: index, page: page, err: err}
	}
}

func (m *transactionModel) resize(width, height int) {
	m.width = max(width, 1)
	m.height = max(height, 1)
	m.viewport.SetWidth(m.contentWidth())
	m.viewport.SetHeight(max(m.height-5, 1))
	if _, ok := m.pages[m.page]; ok {
		m.renderPage(false)
	}
}

func (m *transactionModel) renderPage(resetScroll bool) {
	page, ok := m.pages[m.page]
	if !ok {
		return
	}
	offset := m.viewport.YOffset()
	m.viewport.SetContent(renderTransactions(page.Transactions, m.opts.GroupByMonth, m.contentWidth()))
	if resetScroll {
		m.viewport.GotoTop()
	} else {
		m.viewport.SetYOffset(offset)
	}
}

func (m transactionModel) contentWidth() int {
	return responsiveFrame(m.width, 140).contentWidth
}
