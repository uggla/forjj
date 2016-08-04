package main

import (
    "path"
    "time"
    "fmt"
)

const (
    defaultTimeout = 32 * time.Second
    default_socket_baseurl = "http:///anyhost"
    default_mount_path = "/src"
)
// Start driver task.
func (a *Forj) driver_do(driver_type, action string, args ...string) error {
    d := a.drivers[driver_type] // copy. not a ref.
    a.CurrentPluginDriver = &d

    if err := d.plugin.PluginInit(a.Workspace) ; err != nil {
        return  err
    }

    d.plugin.PluginSetSource(path.Join(a.Workspace_path, a.Workspace, a.w.Infra, "apps", d.driver_type))
    d.plugin.PluginSetWorkspace(path.Join(a.Workspace_path, a.Workspace))
    d.plugin.PluginSocketPath(path.Join(a.Workspace_path, a.Workspace, "lib"))

    if err := d.plugin.PluginStartService() ; err != nil {
        return err
    }

    plugin_args := make(map[string]string)
    a.drivers_options.GetDriversMaintainParameters(plugin_args, action)
    a.GetDriversActionsParameters(plugin_args, "common")
    a.GetDriversActionsParameters(plugin_args, action)

    if res, err := d.plugin.PluginRunAction(action, plugin_args) ; err != nil {
        return  err
    } else {
        d.plugin.Result = res
    }
    fmt.Printf("%s:\n%s\n", d.name, d.plugin.Result.Data.Status)

    // store plugins options required at maintain phase from what the plugin returned.
    a.drivers_options.AddForjjPluginOptions(d.name, d.plugin.Result.Data.Options, driver_type)

    a.drivers[driver_type] = d
    return nil
}

func (a *Forj) driver_cleanup(driver_type string) {
    d := a.drivers[driver_type]
    d.plugin.PluginStopService()
}
