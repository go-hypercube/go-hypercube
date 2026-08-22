package app

import (
	"embed"

	"github.com/go-hypercube/go-hypercube/migration"
)

func (app *App) Migrations() migration.NamespacedSlice { return app.migrations }

func (app *App) RegisterMigration(migrations ...*migration.Migration) error {
	return app.registerMigrationForNamespace(frameworkDevNamespace, migrations...)
}

func (app *App) RegisterMigrationFromFs(files embed.FS) error {
	migrationFiles, err := migration.ExtractFromEmbedFs(files)
	if err != nil {
		return err
	}
	return app.RegisterMigration(migrationFiles...)
}

func (app *App) registerMigrationForNamespace(namespace string, migrations ...*migration.Migration) error {
	namespacedMigrations := make([]*migration.Namespaced, len(migrations))
	for i, m := range migrations {
		namespacedMigrations[i] = migration.NewNamespaced(namespace, m)
	}
	app.migrations = append(app.migrations, namespacedMigrations...)
	return nil
}
