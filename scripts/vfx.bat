@echo off
:: ╔══════════════════════════════════════════════════════╗
:: ║  VampiFox — CMD Wrapper untuk PowerShell script      ║
:: ║  Usage: vfx <command> [arg]                          ║
:: ╚══════════════════════════════════════════════════════╝
powershell -ExecutionPolicy Bypass -File "%~dp0vfx.ps1" %*
