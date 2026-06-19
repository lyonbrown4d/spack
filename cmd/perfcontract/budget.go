package main

import "encoding/json"

func readBudgets(path string) budgetFile {
	body := readRepoFile(path)
	var budgets budgetFile
	if err := json.Unmarshal(body, &budgets); err != nil {
		fatalf("parse budget file: %v", err)
	}
	return budgets
}
