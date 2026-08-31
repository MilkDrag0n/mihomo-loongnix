package cmd

const ManagerServiceUnitTemplate = `[Unit]
Description=mihomo local manager API
After=network.target

[Service]
Type=simple
User=root
UMask=0027
RuntimeDirectory=mihomo-tui
RuntimeDirectoryMode=0750
Environment=MIHOMO_TUI_CORE_SERVICE=mihomo.service
ExecStart={{.ManagerPath}} server -d {{.StateDir}}
Restart=on-failure
RestartSec=3
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`

const CoreServiceUnitTemplate = `[Unit]
Description=mihomo proxy core
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
UMask=0077
ExecStart={{.CorePath}} -d {{.StateDir}} -f {{.ConfigPath}}
Restart=on-failure
RestartSec=3
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`

// ServiceUnitTemplate remains as an alias for downstream source compatibility.
const ServiceUnitTemplate = ManagerServiceUnitTemplate
