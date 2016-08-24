package main

import (
    "fmt"
    "github.hpe.com/christophe-larsonneur/goforjj/trace"
    "log"
)


// Call docker to create the Solution source code from scratch with validated parameters.
// This container do the real stuff (git/call drivers)
// I would expect to have this go tool to have a do_create to replace the shell script.
// But this would be a next version and needs to be validated before this decision is made.
func (a *Forj) Create() error {
    if err := a.define_infra_upstream("create") ; err != nil {
        return fmt.Errorf("Unable to identify a valid infra repository upstream. %s", err)
    }

    gotrace.Trace("Infra upstream selected: '%s'", a.w.Instance)

    // save infra repository location in the workspace.
    defer a.w.Save(a)

    if err, aborted := a.ensure_infra_exists() ; err != nil {
        if !aborted {
            return fmt.Errorf("Failed to ensure infra exists. %s", err)
        }
        log.Printf("Warning. %s", err)
    }

    // Now, we are in the infra repo root directory and at least, the 1st commit exist.

    // Loop on drivers requested like jenkins classified as ci type.
    for instance, _ := range a.drivers {
        defer a.driver_cleanup(instance) // Ensure all instances will be shutted down when done.

        if instance == a.w.Instance {
            continue // Do not try to create infra-upstream twice.
        }
        if err, aborted := a.do_driver_create(instance) ; err != nil {
            if !aborted {
                return fmt.Errorf("Failed to create '%s' source files. %s", instance, err)
            }
            log.Printf("Warning. %s", err)
        }
    }

    log.Print("FORJJ - create ", a.w.Organization, " DONE")
    return nil
}

// This function will ensure minimal git repo exists to store resources plugins data files.
// It will take care of several GIT scenarios. See ensure_local_repo_synced for details
// Used by create action only.
func (a *Forj) ensure_infra_exists() (err error, aborted bool) {

    if err := a.ensure_local_repo_initialized(a.w.Infra) ; err != nil {
        return fmt.Errorf("Unable to ensure infra repository gets initialized. %s.", err), false
    }

    // Now, we are in the infra repo root directory. But at least is completely empty.

    // Set the Initial README.md content for the infra repository.
    a.infra_readme = fmt.Sprintf("Infrastructure Repository for the organization %s", a.Orga_name)

    if a.InfraPluginDriver == nil { // upstream UNdefined.
        if *a.Actions["create"].flagsv["infra-upstream"] != "none"{
            return fmt.Errorf("Your workspace is empty and you did not identified where %s should be pushed (git upstream). To fix this, you have several options:\nYou can confirm that you do not want to configure any upstream with '--infra-upstream none'.\nOr you should define the upstream service with '--app upstream:<UpstreamDriver>[:<InstanceName>]'.\n If you set multiple upstream instances, you will need to connect the appropriate one to the infra repo with '--infra-upstream <InstanceName>'.", a.w.Infra), false
        }

        // Will create the 1st commit and nothing more.
        if err := a.ensure_local_repo_synced(a.w.Infra, "", a.infra_readme) ; err != nil {
            return err, false
        }
        return
    }

    // Upstream defined

    err, aborted = a.do_driver_create(a.w.Instance)
    if aborted {
        // the upstream driver was not able to create the resources because already exist.
        // So the upstream resource may already exist and must be used to restore the local repo content from this resource.
        if e := a.restore_infra_repo() ; err != nil {
            err = fmt.Errorf("%s\n%s", err, e)
        }
    }
    return
}

// Search for upstreams drivers and with or without --infra-upstream setting, the appropriate upstream will define the infra-repo upstream instance to use.
// It sets
// - Forj.w.Instance     : Instance name
func (a *Forj) define_infra_upstream(action string) (err error) {
    // Identify list of upstream instances

    if a.w.Instance != "" { // No need to define infra upstream as loaded from the workspace context.
        return
    }
    infra := a.w.Infra
    a.w.Instance = "none"
    a.infra_upstream = "none"
    upstreams := []*Driver{}
    upstream_requested := *a.Actions[action].flagsv["infra-upstream"]

    if upstream_requested == "none" {
        gotrace.Trace("No upstream instance configured as requested by --infra-upstream none")
        return
    }

    defer func() {
        if d, found := a.drivers[a.w.Instance] ; found {
            d.infraRepo = true
            a.InfraPluginDriver = d
        } else {
            if a.w.Instance != "none" {
                err = fmt.Errorf("Unable to find driver instance '%s' in loaded drivers list.", a.w.Instance)
            }
        }
    }()

    for _, dv := range a.drivers {
        if dv.driver_type == "upstream" {
            upstreams = append(upstreams, dv)
        }
        if dv.name == upstream_requested {
            a.w.Instance = upstream_requested
            return
        }
    }

    if len(upstreams) >1 {
        err = fmt.Errorf("--infra-upstream missing with multiple upstreams defined. please select the appropriate upstream for your Infra repository or 'none'.")
        return
    }

    if len(upstreams) == 1 {
        a.w.Instance = upstreams[0].name
    }
    gotrace.Trace("Selected by default '%s' as upstream instance to connect '%s' repo", a.w.Instance, infra)
    return
}

// Restore the workspace infra repo from the upstream.
func (a *Forj) restore_infra_repo() error {
    if a.InfraPluginDriver.plugin.Result == nil {
        return fmt.Errorf("Internal Error: The infra plugin did not return a valid result. Forj.InfraPluginDriver.plugin.Result = nil.")
    }
    v, found := a.InfraPluginDriver.plugin.Result.Data.Repos[a.w.Infra]

    if  !found {
        return fmt.Errorf("Unable to rebuild your workspace from the upstream '%s'. Not found.", a.w.Infra)
    }

    if ! v.Exist {
        return fmt.Errorf("Unable to rebuild your workspace from the upstream '%s'. Inexistent.", a.w.Infra)
    }

    // Restoring the workspace.
    a.infra_upstream = v.Upstream
    log.Printf("Rebuilding your workspace from '%s(%s)'.", a.w.Infra, a.infra_upstream)
    if err := a.ensure_local_repo_synced(a.w.Infra, a.infra_upstream, a.infra_readme) ; err != nil {
        return fmt.Errorf("infra repository '%s' issue. %s", a.w.Infra)
    }
    log.Printf("Note: As your workspace was empty, it has been rebuilt from '%s'. \nUse create to create new application sources, or update to update existing application sources", a.infra_upstream)
    return nil
}

// Execute the driver task, with commit/push and will execute the maintain step.
func (a *Forj) do_driver_create(instance string) (err error, aborted bool) {
    // Calling upstream driver - To create plugin source files for the current upstream infra repository
    if err, aborted = a.driver_do(instance, "create") ; err != nil {
        return
    }

    if a.InfraPluginDriver != nil && a.infra_upstream == "none" {
        if v, found := a.InfraPluginDriver.plugin.Result.Data.Repos[a.w.Infra] ; found {
            a.infra_upstream = v.Upstream
        } else {
            return fmt.Errorf("Unable to find '%s' from driver '%s'", a.w.Infra, a.w.Instance), false
        }
    }

    // Ensure initial commit exists and upstream are set for the infra repository
    if err := a.ensure_local_repo_synced(a.w.Infra, a.infra_upstream, a.infra_readme) ; err != nil {
        return fmt.Errorf("infra repository '%s' issue. %s", err), false
    }

    // Add source files
    if err := a.CurrentPluginDriver.gitAddPluginFiles() ; err != nil {
        return fmt.Errorf("Issue to add driver '%s' generated files. %s", a.CurrentPluginDriver.name, err), false
    }

    // Commit files and drivers options
    if err := a.CurrentPluginDriver.gitCommit() ; err != nil {
        return fmt.Errorf("git commit issue. %s", err), false
    }

    if a.infra_upstream != "none" {
        if err := gitPush() ; err != nil {
            return err, false
        }
    }

    // TODO: Except if --no-maintain is set, we could just create files and do maintain later.
    if err := a.do_driver_maintain(instance) ; err != nil { // This will create/configure the upstream service
        return err, false
    }
    return
}