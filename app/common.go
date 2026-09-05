package app

// hostAppNamespace is the reserved namespace under which commands,
// migrations, and seeders registered directly by the developer using
// this framework (the "host application") are grouped — as opposed to
// those contributed by a plugin via UsePlugin.
//
// It is used as the namespace argument when RegisterCommand,
// RegisterMigration, RegisterMigrationFromFs, and RegisterSeeder wrap
// their input in a cmd.Namespaced / migration.Namespaced /
// seeder.Namespaced before appending it to the app's respective
// registries (app.cmds, app.migrations, app.seeders). This keeps
// host-app-owned entries grouped together and distinguishable from
// plugin-owned ones when querying by namespace (e.g. via
// NamespacedSlice.GetNamespace, GroupByNamespace, or Namespaces).
//
// The name "owner" is reserved: UsePlugin rejects any plugin whose
// Name() equals hostAppNamespace, returning an error rather than
// allowing a plugin to collide with (and therefore be indistinguishable
// from) the host application's own namespace.
const hostAppNamespace = "owner"
