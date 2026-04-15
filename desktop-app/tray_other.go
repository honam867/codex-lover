//go:build !windows

package main

type noopTray struct{}

func (a *App) startTray() {}

func (n *noopTray) Update(Snapshot) {}

func (n *noopTray) Close() {}
