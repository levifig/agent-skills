package cli

import (
	"github.com/levifig/loaf/internal/state"
)

func enrichReportList(status state.Status, reports *state.ReportList) {
	if status.Mode == state.ModeMarkdownOnly {
		return
	}
	reports.ContractVersion = state.StateJSONContractVersion
	reports.DatabaseScope = status.DatabaseScope
	reports.DatabasePath = status.DatabasePath
	reports.ProjectID = status.ProjectID
	reports.ProjectName = status.ProjectName
	reports.ProjectCurrentPath = status.ProjectCurrentPath
}

func enrichReportShow(status state.Status, result *state.ReportShow) {
	if status.Mode == state.ModeMarkdownOnly {
		return
	}
	result.ContractVersion = state.StateJSONContractVersion
	result.DatabaseScope = status.DatabaseScope
	result.DatabasePath = status.DatabasePath
	result.ProjectID = status.ProjectID
	result.ProjectName = status.ProjectName
	result.ProjectCurrentPath = status.ProjectCurrentPath
}

func enrichReportCreate(status state.Status, result *state.ReportCreateResult) {
	if status.Mode == state.ModeMarkdownOnly {
		return
	}
	result.ContractVersion = state.StateJSONContractVersion
	result.DatabaseScope = status.DatabaseScope
	result.DatabasePath = status.DatabasePath
	result.ProjectID = status.ProjectID
	result.ProjectName = status.ProjectName
	result.ProjectCurrentPath = status.ProjectCurrentPath
}

func enrichReportStatus(status state.Status, result *state.ReportStatusResult) {
	if status.Mode == state.ModeMarkdownOnly {
		return
	}
	result.ContractVersion = state.StateJSONContractVersion
	result.DatabaseScope = status.DatabaseScope
	result.DatabasePath = status.DatabasePath
	result.ProjectID = status.ProjectID
	result.ProjectName = status.ProjectName
	result.ProjectCurrentPath = status.ProjectCurrentPath
}
