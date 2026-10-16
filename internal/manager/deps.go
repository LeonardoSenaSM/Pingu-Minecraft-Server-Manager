package manager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"mineserver/internal/api"
)

const (
	stagingDirName = ".pingu-staging"
	keepJarBackups = 3
)

// dependency descreve um arquivo gerenciado pelo Pingu. Os opcionais são
// pulados (mantendo o arquivo atual) quando não há versão compatível.
type dependency struct {
	name     string
	relative string
	optional bool
	resolve  func(ctx context.Context, minecraftVersion string) (api.Artifact, error)
}

type stagedFile struct {
	dep      dependency
	artifact api.Artifact
	staged   string
}

func (m *Manager) pluginDependencies() []dependency {
	return []dependency{
		{name: "Geyser-Spigot", relative: filepath.Join("plugins", "Geyser-Spigot.jar"), resolve: func(ctx context.Context, _ string) (api.Artifact, error) {
			return m.api.GeyserArtifact(ctx, "geyser")
		}},
		{name: "Floodgate-Spigot", relative: filepath.Join("plugins", "Floodgate-Spigot.jar"), resolve: func(ctx context.Context, _ string) (api.Artifact, error) {
			return m.api.GeyserArtifact(ctx, "floodgate")
		}},
		{name: "Chunky", relative: filepath.Join("plugins", "Chunky.jar"), optional: true, resolve: func(ctx context.Context, version string) (api.Artifact, error) {
			return m.api.ModrinthArtifact(ctx, "chunky", version)
		}},
		{name: "CoreProtect", relative: filepath.Join("plugins", "CoreProtect.jar"), optional: true, resolve: func(ctx context.Context, version string) (api.Artifact, error) {
			return m.api.ModrinthArtifact(ctx, "coreprotect", version)
		}},
	}
}

// dependenciesPresent considera apenas os arquivos obrigatórios.
func (m *Manager) dependenciesPresent() bool {
	required := []string{"server.jar"}
	for _, dep := range m.pluginDependencies() {
		if !dep.optional {
			required = append(required, dep.relative)
		}
	}
	for _, relative := range required {
		if info, err := os.Stat(filepath.Join(m.serverDir, relative)); err != nil || info.Size() == 0 {
			return false
		}
	}
	return true
}

func (m *Manager) AvailablePaperVersions(ctx context.Context) ([]string, error) {
	return m.api.PaperVersions(ctx)
}

// UpdatePaperVersion troca a versão do Minecraft. Atualiza o conjunto inteiro
// (Paper e plugins), porque trocar só o Paper deixaria plugins incompatíveis.
func (m *Manager) UpdatePaperVersion(ctx context.Context) error {
	return m.UpdateDependencies(ctx)
}

// UpdateDependencies atualiza Paper e plugins como uma transação:
//  1. resolve todas as versões antes de tocar em qualquer arquivo;
//  2. baixa tudo numa pasta temporária, conferindo checksum e integridade;
//  3. faz backup do mundo se a versão do Minecraft mudar;
//  4. move os arquivos atuais para server/backups e instala os novos;
//  5. se qualquer passo da instalação falhar, restaura os anteriores.
func (m *Manager) UpdateDependencies(ctx context.Context) (result error) {
	m.dependenciesMu.Lock()
	defer m.dependenciesMu.Unlock()
	const totalTasks = 5
	defer func() {
		progress := Progress{Done: true, TotalTasks: totalTasks}
		if result != nil {
			progress.Err = result.Error()
			m.Log("error", "dependencies", "atualização não concluída", result)
		}
		m.notifyProgress(progress)
	}()
	if ctx == nil {
		ctx = m.ctx
	}
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if m.currentProcess() != nil {
		return errors.New("pare o servidor antes de atualizar os arquivos")
	}
	if err := m.ensureDirectories(); err != nil {
		return err
	}

	m.notifyProgress(Progress{Task: "PaperMC", TotalTasks: totalTasks, Indeterminate: true})
	paper, err := m.api.ResolvePaper(ctx, m.SelectedVersion(), m.Settings().AllowExperimentalBuilds)
	if err != nil {
		return fmt.Errorf("localizar PaperMC: %w", err)
	}
	if paper.Channel != "" && paper.Channel != api.ChannelStable {
		m.Log("warning", "dependencies", fmt.Sprintf("usando build de teste do PaperMC: Minecraft %s, build %d (%s). Faça backups com frequência; quando sair o build estável, use \"Atualizar dependências\"", paper.Version, paper.Build, paper.Channel), nil)
	}
	plan := []stagedFile{{dep: dependency{name: "PaperMC", relative: "server.jar"}, artifact: paper}}
	for _, dep := range m.pluginDependencies() {
		artifact, err := dep.resolve(ctx, paper.Version)
		if err != nil {
			if dep.optional && errors.Is(err, api.ErrNoCompatibleVersion) {
				m.Log("warning", "dependencies", fmt.Sprintf("%s ainda não tem versão para Minecraft %s; o arquivo atual será mantido", dep.name, paper.Version), nil)
				continue
			}
			return fmt.Errorf("localizar %s: %w", dep.name, err)
		}
		plan = append(plan, stagedFile{dep: dep, artifact: artifact})
	}

	staging := filepath.Join(m.serverDir, stagingDirName)
	if err := os.RemoveAll(staging); err != nil {
		return fmt.Errorf("limpar pasta temporária: %w", err)
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := m.downloadAll(ctx, staging, plan); err != nil {
		return err
	}

	installed := m.InstalledPaperVersion()
	if installed != "" && installed != paper.Version {
		m.notifyProgress(Progress{Task: "Backup", TotalTasks: totalTasks, Indeterminate: true})
		backup, err := m.backupWorld(ctx, installed, paper.Version)
		if err != nil {
			return fmt.Errorf("backup do mundo antes de trocar de versão: %w", err)
		}
		if backup != "" {
			m.Log("info", "backup", "backup do mundo criado em "+backup, nil)
		}
	}
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if err := m.installStaged(plan); err != nil {
		return err
	}
	if err := m.writeInstallation(PaperInstallation{Version: paper.Version, Channel: paper.Channel, Build: paper.Build, DownloadedAt: time.Now()}); err != nil {
		return err
	}
	if err := m.configureGeyser(m.geyserPort(), m.paperPort()); err != nil {
		return err
	}
	if err := pruneBackups(m.backupsDir(), "jars-", keepJarBackups); err != nil {
		m.Log("warning", "backup", "não foi possível remover backups antigos de JARs", err)
	}
	for _, item := range plan {
		version := item.artifact.Version
		if version == "" {
			version = "?"
		}
		m.Log("info", "dependencies", fmt.Sprintf("%s %s instalado", item.dep.name, version), nil)
	}
	m.notifyStatus()
	m.notifyFiles()
	return nil
}

func (m *Manager) downloadAll(ctx context.Context, staging string, plan []stagedFile) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		wait        sync.WaitGroup
		mu          sync.Mutex
		completed   int
		downloadErr []error
	)
	for index := range plan {
		item := &plan[index]
		item.staged = filepath.Join(staging, filepath.Base(item.dep.relative))
		wait.Add(1)
		go func() {
			defer wait.Done()
			m.Log("info", "download", "baixando "+item.dep.name, nil)
			err := m.api.Download(ctx, item.artifact, item.staged, func(downloaded, total int64) {
				mu.Lock()
				current := completed
				mu.Unlock()
				m.notifyProgress(Progress{Task: item.dep.name, Downloaded: downloaded, Total: total, Completed: current, TotalTasks: len(plan), Indeterminate: total <= 0})
			})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				downloadErr = append(downloadErr, fmt.Errorf("baixar %s: %w", item.dep.name, err))
				cancel()
				return
			}
			completed++
		}()
	}
	wait.Wait()
	return errors.Join(downloadErr...)
}

// installStaged move os arquivos atuais para um backup e coloca os novos no
// lugar. Em caso de falha, desfaz o que já tinha sido trocado.
func (m *Manager) installStaged(plan []stagedFile) error {
	backupDir := filepath.Join(m.backupsDir(), "jars-"+time.Now().Format("20060102-150405"))
	type done struct{ destination, backup string }
	var applied []done
	rollback := func(cause error) error {
		var errs []error
		for index := len(applied) - 1; index >= 0; index-- {
			step := applied[index]
			if err := os.Remove(step.destination); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, err)
			}
			if step.backup != "" {
				if err := os.Rename(step.backup, step.destination); err != nil {
					errs = append(errs, err)
				}
			}
		}
		if len(errs) > 0 {
			return fmt.Errorf("%w; a restauração também falhou (%v) — os arquivos anteriores estão em %s", cause, errors.Join(errs...), backupDir)
		}
		return fmt.Errorf("%w; os arquivos anteriores foram restaurados", cause)
	}
	for _, item := range plan {
		destination := filepath.Join(m.serverDir, item.dep.relative)
		step := done{destination: destination}
		if _, err := os.Stat(destination); err == nil {
			step.backup = filepath.Join(backupDir, item.dep.relative)
			if err := os.MkdirAll(filepath.Dir(step.backup), 0o755); err != nil {
				return rollback(err)
			}
			if err := os.Rename(destination, step.backup); err != nil {
				return rollback(fmt.Errorf("guardar %s: %w", item.dep.name, err))
			}
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			applied = append(applied, step)
			return rollback(err)
		}
		if err := os.Rename(item.staged, destination); err != nil {
			applied = append(applied, step)
			return rollback(fmt.Errorf("instalar %s: %w", item.dep.name, err))
		}
		applied = append(applied, step)
	}
	return nil
}
