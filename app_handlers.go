package main

import (
	"net/http"
)

func (a *App) handleMessages(w http.ResponseWriter, r *http.Request) {
	state := a.CurrentState()
	a.requestCount.Add(1)
	state.HandleMessages(w, r)
}

func (a *App) handleModels(w http.ResponseWriter, r *http.Request) {
	state := a.CurrentState()
	state.HandleModels(w, r)
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	state := a.CurrentState()
	state.HandleHealth(w, r)
}
