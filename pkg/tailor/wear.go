package tailor

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pieroproietti/penguins-tailor/pkg/distro"
	"github.com/pieroproietti/penguins-tailor/pkg/utils"
)

var (
	newWearDistro         = distro.NewDistro
	newWearPackageManager = newPackageManager
	ensureWearHeaders     = ensureKernelHeaders
	applyWearSuit         = applySuit
	getWearWardrobeRoot   = getWardrobeRoot
	getWearWardrobeV2Dir  = getWardrobeV2Dir
	promptWearConfirm     = promptConfirm
	applyWearSysroot      = applySysroot
	copyWearSkelToUser    = copySkelToUser
)

func Wear(costumeName string, linear bool, branch string, dryRun bool) error {
	d := newWearDistro()
	systemIdentity := d.Identity()

	if os.Geteuid() != 0 && !dryRun {
		utils.LogError("'tailor wear' needs to install packages and write to system paths; run it as root (e.g. 'sudo tailor wear %s').", costumeName)
		return fmt.Errorf("must be run as root")
	}

	pm, errPm := newWearPackageManager(d.FamilyID)
	if errPm != nil {
		distroName := d.DistroID
		if distroName == "" {
			distroName = d.FamilyID
		}
		utils.LogWarning("Distribution '%s' (family: %s) is not supported.", distroName, d.FamilyID)
		if !promptWearConfirm("Do you want to copy sysroot to / and configure the user home directory?") {
			utils.LogError("Distribution '%s' (family: %s) is not supported. Tailor currently supports Debian derivatives.", distroName, d.FamilyID)
			return fmt.Errorf("unsupported distribution family: %s", d.FamilyID)
		}
		return wearSysrootOnly(costumeName, linear, branch, dryRun, systemIdentity)
	}

	if err := pm.Refresh(); err != nil {
		return fmt.Errorf("failed to refresh package metadata: %w", err)
	}

	root, err := getWearWardrobeRoot()
	if err != nil {
		utils.LogError("Wardrobe root error: %v", err)
		return err
	}

	// If branch is specified, or if the wardrobe repository does not exist yet (and we're not in local ./v2 dev mode),
	// ensure wardrobe is fetched/cloned and on the right branch.
	if branch != "" {
		if err := Get("", branch); err != nil {
			return fmt.Errorf("failed to get costumes repository on branch '%s': %w", branch, err)
		}
	} else if _, errStat := os.Stat(root); os.IsNotExist(errStat) {
		if stat, errV2 := os.Stat("v2"); errV2 != nil || !stat.IsDir() {
			if err := Get("", ""); err != nil {
				return fmt.Errorf("failed to download costumes repository: %w", err)
			}
		}
	}

	v2Dir, err := getWearWardrobeV2Dir()
	if err != nil {
		utils.LogError("Wardrobe root error: %v", err)
		return err
	}
	costumeDir, err := resolveCostumeDir(v2Dir, costumeName)
	if err != nil {
		return err
	}

	yamlFile := findYaml(costumeDir)
	suit, err := loadSuit(yamlFile)
	if err != nil {
		return err
	}

	// Enforce distribution compatibility BEFORE anything else
	if err := checkCostumeCompatibility(costumeDir, suit); err != nil {
		utils.LogError("%s", incompatibleDistroMessage(suit.Name, suit.Distributions, currentDistroName()))
		return err
	}

	isDirectAccessory := strings.HasPrefix(costumeName, "accessories/") || (suit.Name != "" && !strings.Contains(costumeDir, "/costumes/"))
	if !isDirectAccessory {
		if err := validateBranding(v2Dir, suit.Branding); err != nil {
			return err
		}
	}

	origin := GetWardrobeOrigin()
	activeBranch := GetWardrobeBranch()

	costumeLabel := fmt.Sprintf("Costume: %s", suit.Name)
	if suit.Release != "" {
		costumeLabel = fmt.Sprintf("Costume: %s (v%s)", suit.Name, suit.Release)
	}
	if isDirectAccessory {
		costumeLabel = fmt.Sprintf("Accessory: %s", suit.Name)
		if suit.Release != "" {
			costumeLabel = fmt.Sprintf("Accessory: %s (v%s)", suit.Name, suit.Release)
		}
	}

	notes := suit.Description
	if findPreseed(costumeDir) != "" {
		if notes != "" {
			notes += " - Preseed applied"
		} else {
			notes = "Preseed applied"
		}
	}
	if dryRun {
		if notes != "" {
			notes = "[DRY-RUN] " + notes
		} else {
			notes = "[DRY-RUN] Simulation mode (no changes will be applied)"
		}
	}

	headerCfg := utils.SplitScreenConfig{
		Atelier: origin,
		System:  systemIdentity,
		Costume: costumeLabel,
		Branch:  activeBranch,
		Notes:   notes,
	}

	var ss *utils.SplitScreen
	if !linear {
		ss = utils.StartSplitScreenConfig(headerCfg)
		if ss != nil {
			defer ss.Close()
		}
	}
	if ss == nil {
		utils.PrintBannerConfig(headerCfg)
	}

	SetLicensePromptPackages(suit.PackagesInteractive)

	if ss != nil {
		costumeActionTag := "Costume"
		if isDirectAccessory {
			costumeActionTag = "Accessory"
		}
		ss.SetAction("%s: %s...", costumeActionTag, suit.Name)
	}

	packageResult, err := applyWearSuit(costumeDir, suit, dryRun, false, pm)
	if err != nil {
		return err
	}
	installedPackages := packageResult.Installed
	unavailablePackages := packageResult.Unavailable
	failedPackages := packageResult.Failed

	costumePreseedSuffix := ""
	if findPreseed(costumeDir) != "" {
		costumePreseedSuffix = " - Preseed applied"
	}
	costumeDetails := ""
	if len(installedPackages) > 0 || len(failedPackages) > 0 {
		if len(failedPackages) > 0 {
			costumeDetails = fmt.Sprintf(" - Installed %d packages (%d failed)", len(installedPackages), len(failedPackages))
		} else {
			suffix := ""
			if dryRun {
				suffix = " (simulated)"
			}
			costumeDetails = fmt.Sprintf(" - Installed %d packages%s", len(installedPackages), suffix)
		}
	}

	costumeTag := "Costume"
	if isDirectAccessory {
		costumeTag = "Accessory"
	}
	displayName := suit.Name
	if displayName == "" {
		displayName = costumeName
	}

	if ss != nil {
		ss.SetPackageSummary(len(installedPackages), len(unavailablePackages), len(failedPackages))
		ss.AddPackageStep(fmt.Sprintf("%s: %s%s", costumeTag, displayName, costumePreseedSuffix), len(packageResult.Installed), len(packageResult.Unavailable), len(packageResult.Failed))
	} else {
		utils.PrintSubSection("-->", fmt.Sprintf("%s: %s%s%s", costumeTag, displayName, costumeDetails, costumePreseedSuffix))
	}

	if len(suit.Accessories) > 0 {
		if ss == nil {
			utils.PrintSection("👝", fmt.Sprintf("ACCESSORIES (%d items)", len(suit.Accessories)))
		}
		for idx, accName := range suit.Accessories {
			accDir := resolveAccessoryDir(v2Dir, costumeDir, accName)

			if accYaml := findYaml(accDir); accYaml != "" {
				if accSuit, err := loadSuit(accYaml); err == nil {
					if ss != nil {
						ss.SetAction("Accessory [%d/%d]: %s...", idx+1, len(suit.Accessories), accName)
					}
					accResult, _ := applyWearSuit(accDir, accSuit, dryRun, true, pm)
					installedPackages = append(installedPackages, accResult.Installed...)
					unavailablePackages = append(unavailablePackages, accResult.Unavailable...)
					failedPackages = append(failedPackages, accResult.Failed...)
					if ss != nil {
						ss.SetPackageSummary(len(installedPackages), len(unavailablePackages), len(failedPackages))
					}

					// If the accessory defines nested accessories, apply them recursively
					if len(accSuit.Accessories) > 0 {
						for subIdx, subAccName := range accSuit.Accessories {
							subAccDir := resolveAccessoryDir(v2Dir, accDir, subAccName)
							if subAccYaml := findYaml(subAccDir); subAccYaml != "" {
								if subAccSuit, err := loadSuit(subAccYaml); err == nil {
									if ss != nil {
										ss.SetAction("  Nested accessory [%d/%d]: %s...", subIdx+1, len(accSuit.Accessories), subAccName)
									}
									subResult, _ := applyWearSuit(subAccDir, subAccSuit, dryRun, true, pm)
									installedPackages = append(installedPackages, subResult.Installed...)
									unavailablePackages = append(unavailablePackages, subResult.Unavailable...)
									failedPackages = append(failedPackages, subResult.Failed...)
									if ss != nil {
										ss.SetPackageSummary(len(installedPackages), len(unavailablePackages), len(failedPackages))
										ss.AddPackageStep(fmt.Sprintf("Nested accessory [%d/%d]: %s", subIdx+1, len(accSuit.Accessories), subAccName), len(subResult.Installed), len(subResult.Unavailable), len(subResult.Failed))
									}
									if len(subAccSuit.FinalizeCmds) > 0 {
										executeFinalizeCommands(subAccSuit.FinalizeCmds, subAccDir, subAccSuit.Name, dryRun)
									}
								}
							}
						}
					}

					// If the accessory defines its own FinalizeCmds, execute them for this accessory
					if len(accSuit.FinalizeCmds) > 0 {
						executeFinalizeCommands(accSuit.FinalizeCmds, accDir, accSuit.Name, dryRun)
					}

					accPreseedSuffix := ""
					if findPreseed(accDir) != "" {
						accPreseedSuffix = " - Preseed applied"
					}
					accDetails := ""
					if len(accResult.Installed) > 0 || len(accResult.Failed) > 0 {
						if len(accResult.Failed) > 0 {
							accDetails = fmt.Sprintf(" - Installed %d packages (%d failed)", len(accResult.Installed), len(accResult.Failed))
						} else {
							suffix := ""
							if dryRun {
								suffix = " (simulated)"
							}
							accDetails = fmt.Sprintf(" - Installed %d packages%s", len(accResult.Installed), suffix)
						}
					}

					if ss != nil {
						ss.SetPackageSummary(len(installedPackages), len(unavailablePackages), len(failedPackages))
						ss.AddPackageStep(fmt.Sprintf("Accessory [%d/%d]: %s%s", idx+1, len(suit.Accessories), accName, accPreseedSuffix), len(accResult.Installed), len(accResult.Unavailable), len(accResult.Failed))
					} else {
						utils.PrintSubSection("-->", fmt.Sprintf("[%d/%d] Accessory: %s%s%s", idx+1, len(suit.Accessories), accName, accDetails, accPreseedSuffix))
					}
				} else {
					if ss != nil {
						ss.AddStep(fmt.Sprintf("%s[WARN]%s Could not load accessory '%s'", utils.ColorYellow, utils.ColorReset, accName))
					} else {
						fmt.Printf("  %s[WARN] Could not load accessory '%s': %v%s\n", utils.ColorYellow, accName, err, utils.ColorReset)
					}
				}
			} else {
				if ss != nil {
					ss.AddStep(fmt.Sprintf("%s[WARN]%s Accessory '%s' not found", utils.ColorYellow, utils.ColorReset, accName))
				} else {
					fmt.Printf("  %s[WARN] Accessory '%s' not found in %s%s\n", utils.ColorYellow, accName, accDir, utils.ColorReset)
				}
			}
		}
	}

	// DKMS healing
	if !dryRun {
		if ss != nil {
			ss.SetAction("Healing DKMS state...")
		}
		failedPackages = healAndRetryFailed(failedPackages, pm)
		if ss != nil {
			ss.SetPackageSummary(len(installedPackages), len(unavailablePackages), len(failedPackages))
		}
	}

	// Costume Sysroot Overlay
	applyWearSysroot(costumeDir, suit.Name, dryRun, false)

	// Costume Finalization commands
	if len(suit.FinalizeCmds) > 0 {
		executeFinalizeCommands(suit.FinalizeCmds, costumeDir, suit.Name, dryRun)
	}

	// Reconcile the active branding only when wearing a costume. Applying an
	// accessory directly must not change the identity selected by the costume.
	if !isDirectAccessory {
		if err := applyBranding(v2Dir, suit.Branding, dryRun); err != nil {
			return err
		}
	}

	// User environment synchronization
	targetUser := getTargetUsername()
	if targetUser != "" && targetUser != "root" {
		if ss != nil {
			ss.SetAction("Synchronizing user environment (/etc/skel -> /home/%s)...", targetUser)
		}
		copyWearSkelToUser(dryRun)
		if ss != nil {
			statusMsg := fmt.Sprintf("User environment synchronized (%s)", targetUser)
			if dryRun {
				statusMsg += " (simulated)"
			}
			ss.AddStep(fmt.Sprintf("%s[OK]%s %s", utils.ColorGreen, utils.ColorReset, statusMsg))
		}
	}

	// Close split screen before printing final summary box
	if ss != nil {
		ss.Close()
		ss = nil
	}

	reportPath, reportErr := writeWearReport(wearReport{
		CostumeName:   suit.Name,
		System:        systemIdentity,
		Installed:     installedPackages,
		Unavailable:   unavailablePackages,
		FailedInstall: failedPackages,
	})

	costumeSummaryVal := suit.Name
	if dryRun {
		costumeSummaryVal = fmt.Sprintf("%s [DRY-RUN]", suit.Name)
	}
	summaryRows := [][2]string{
		{"Costume / Item", costumeSummaryVal},
	}
	if origin != "" {
		atelierVal := origin
		if activeBranch != "" && activeBranch != "main" && activeBranch != "master" {
			atelierVal = fmt.Sprintf("%s (%s)", origin, activeBranch)
		}
		summaryRows = append(summaryRows, [2]string{"Atelier", atelierVal})
	}
	pkgInstalledLabel := "Packages installed"
	if dryRun {
		pkgInstalledLabel = "Packages to install"
	}
	summaryRows = append(summaryRows,
		[2]string{pkgInstalledLabel, fmt.Sprintf("%d", len(installedPackages))},
		[2]string{"Packages NOT installed", fmt.Sprintf("%d", len(failedPackages))},
	)
	if reportErr == nil {
		summaryRows = append(summaryRows, [2]string{"Detailed report", reportPath})
	}
	if !dryRun {
		summaryRows = append(summaryRows, [2]string{"System log", tailorLogFile})
	}

	summaryTitle := "✨ WEAR COMPLETED!"
	if dryRun {
		summaryTitle = "✨ WEAR COMPLETED (SIMULATION / DRY-RUN)!"
	}
	utils.PrintSummaryBox(summaryTitle, summaryRows)

	if !dryRun {
		printKernelCleanupReminder()
		if suit.DisplayManagerNotice {
			printDisplayManagerNotice()
		}
		if suit.Reboot {
			rebootSystem()
		}
	}
	return nil
}

func getTargetUsername() string {
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		return sudoUser
	}
	if u := firstHumanUser(); u != nil {
		return u.Username
	}
	return ""
}

// checkCostumeCompatibility enforces the distribution compatibility declared by the costume.
func checkCostumeCompatibility(costumeDir string, suit *Suit) error {
	if len(suit.Distributions) == 0 {
		return nil
	}

	current := currentDistroCodename()
	if current == "" {
		logToFile("WARNING: could not detect the running distribution; skipping compatibility check.")
		return nil
	}

	for _, d := range suit.Distributions {
		if strings.EqualFold(strings.TrimSpace(d), current) {
			return nil
		}
	}

	script := filepath.Join(costumeDir, "tailor-check")
	if _, err := os.Stat(script); os.IsNotExist(err) {
		script = filepath.Join(costumeDir, "wardrobe-check")
	}
	if _, err := os.Stat(script); err == nil {
		cmd := exec.Command("bash", script)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
	} else {
		fmt.Fprintf(os.Stderr,
			"This costume (%s) is only compatible with: %s. Detected distribution: %s.\n",
			suit.Name, strings.Join(suit.Distributions, ", "), current)
	}

	return fmt.Errorf("aborted: distribution %q not supported", current)
}

// currentDistroCodename reads /etc/os-release and returns the VERSION_CODENAME
func currentDistroCodename() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "VERSION_CODENAME=") {
			return strings.Trim(strings.TrimPrefix(line, "VERSION_CODENAME="), `"`)
		}
	}
	return ""
}

// ensureKernelHeaders installs the kernel headers matching the currently running kernel.
func ensureKernelHeaders(dryRun bool) error {
	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		logToFile(fmt.Sprintf("WARNING: could not determine running kernel version: %v", err))
		return err
	}
	release := strings.TrimSpace(string(out))
	if release == "" {
		return nil
	}
	archOut, _ := exec.Command("dpkg", "--print-architecture").Output()
	arch := strings.TrimSpace(string(archOut))
	if arch == "" {
		arch = "amd64"
	}
	pkgs := fmt.Sprintf("linux-headers-%s linux-headers-%s", release, arch)
	if dryRun {
		logToFile(fmt.Sprintf("[DRY-RUN] Would ensure kernel headers are present: %s", pkgs))
		return nil
	}
	logToFile(fmt.Sprintf("Ensuring kernel headers are present before DKMS installs: %s", pkgs))
	return utils.ExecLogOnly("UCF_FORCE_CONFFOLD=1 DEBIAN_FRONTEND=readline apt-get install -o Dpkg::Options::='--force-confdef' -o Dpkg::Options::='--force-confold' -y "+pkgs, tailorLogFile)
}

// healAndRetryFailed repairs the half-configured dpkg state and retries failed packages.
func healAndRetryFailed(failed []string, pm PackageManager) []string {
	if len(failed) == 0 {
		return nil
	}

	logToFile("Healing dpkg state before retrying failed packages...")
	_ = pm.Heal()

	logToFile(fmt.Sprintf("Retrying %d packages now that kernel headers are in place...", len(failed)))
	pm.Install(failed, InstallMode{Retries: 1})

	var still []string
	for _, p := range failed {
		if !pm.IsInstalled(p) {
			still = append(still, p)
		}
	}
	return still
}

// applySuit applies a costume or accessory definition with clean spinners
func applySuit(dir string, suit *Suit, dryRun bool, isAccessory bool, pm PackageManager) (PackageInstallResult, error) {
	var result PackageInstallResult
	ss := utils.GetSplitScreen()

	// Preseed (debconf-set-selections)
	if preseedFile := findPreseed(dir); preseedFile != "" {
		if ss != nil {
			ss.SetAction("Applying preseed selections (%s)...", filepath.Base(preseedFile))
		}
		if dryRun {
			logToFile(fmt.Sprintf("[DRY-RUN] Would apply preseed: %s", preseedFile))
		} else {
			if err := applyPreseed(preseedFile, suit.Name); err != nil {
				logToFile(WarnPrefix(suit.Name) + fmt.Sprintf("Preseed application warning: %v", err))
			}
		}
	}

	// Repositories
	if suit.Sequence != nil && suit.Sequence.Repositories != nil {
		if ss != nil {
			if suit.Name != "" {
				ss.SetAction("%s, configuring package repositories & updating cache...", suit.Name)
			} else {
				ss.SetAction("Configuring package repositories & updating cache...")
			}
		}
		setupRepositories(suit.Sequence.Repositories, suit.Name, dryRun, pm)
		if !isAccessory && ss != nil {
			statusMsg := "Repositories configured & updated"
			if dryRun {
				statusMsg += " (simulated)"
			}
			ss.AddStep(fmt.Sprintf("%s[OK]%s %s", utils.ColorGreen, utils.ColorReset, statusMsg))
		}
	}

	// DKMS safety: resolve the running kernel headers only after the costume's
	// repositories and package metadata have been configured.
	if !isAccessory {
		checkKernelHeaders(dryRun, ss)
	}

	// Packages
	if len(suit.Packages) > 0 {
		if ss != nil {
			if suit.Name != "" {
				ss.SetAction("%s, installing packages (%d packages)...", suit.Name, len(suit.Packages))
			} else {
				ss.SetAction("Installing packages (%d packages)...", len(suit.Packages))
			}
		}
		if dryRun {
			logToFile(fmt.Sprintf("[DRY-RUN] Would install %d packages: %v", len(suit.Packages), suit.Packages))
			result.Installed = append(result.Installed, suit.Packages...)
		} else {
			result.merge(pm.Install(suit.Packages, InstallMode{Retries: 3}))
		}
	}

	// Packages No Recommends
	if len(suit.PackagesNoRecommends) > 0 {
		if ss != nil {
			if suit.Name != "" {
				ss.SetAction("%s, installing packages without recommends (%d packages)...", suit.Name, len(suit.PackagesNoRecommends))
			} else {
				ss.SetAction("Installing packages without recommends (%d packages)...", len(suit.PackagesNoRecommends))
			}
		}
		if dryRun {
			logToFile(fmt.Sprintf("[DRY-RUN] Would install %d packages without recommends: %v", len(suit.PackagesNoRecommends), suit.PackagesNoRecommends))
			result.Installed = append(result.Installed, suit.PackagesNoRecommends...)
		} else {
			result.merge(pm.Install(suit.PackagesNoRecommends, InstallMode{NoRecommends: true, Retries: 3}))
		}
	}

	// Packages Interactive
	if len(suit.PackagesInteractive) > 0 {
		if ss != nil {
			if suit.Name != "" {
				ss.SetAction("%s, configuring %d interactive packages...", suit.Name, len(suit.PackagesInteractive))
			} else {
				ss.SetAction("Configuring %d interactive packages...", len(suit.PackagesInteractive))
			}
		}
		if dryRun {
			logToFile(fmt.Sprintf("[DRY-RUN] Would configure %d interactive packages: %v", len(suit.PackagesInteractive), suit.PackagesInteractive))
			result.Installed = append(result.Installed, suit.PackagesInteractive...)
		} else {
			result.merge(pm.Install(suit.PackagesInteractive, InstallMode{Interactive: true}))
		}
	}

	// For accessories, apply their sysroot overlay right after their packages
	if isAccessory {
		applyWearSysroot(dir, suit.Name, dryRun, true)
	}

	// Sequence commands (intermediate commands defined in sequence.cmds)
	if len(suit.SequenceCmds) > 0 {
		if ss != nil {
			if suit.Name != "" {
				ss.SetAction("%s, running sequence scripts (%d commands)...", suit.Name, len(suit.SequenceCmds))
			} else {
				ss.SetAction("Running sequence scripts (%d commands)...", len(suit.SequenceCmds))
			}
		}
		for idx, command := range suit.SequenceCmds {
			fields := strings.Fields(command)
			cmdName := command
			if len(fields) > 0 {
				cmdName = filepath.Base(fields[0])
			}
			if ss != nil {
				if suit.Name != "" {
					ss.SetAction("%s, [%d/%d] Sequence: %s", suit.Name, idx+1, len(suit.SequenceCmds), cmdName)
				} else {
					ss.SetAction("[%d/%d] Sequence: %s", idx+1, len(suit.SequenceCmds), cmdName)
				}
			}
			if dryRun {
				logToFile(fmt.Sprintf("[DRY-RUN] Would run sequence command: %s", command))
				continue
			}
			if len(fields) > 0 {
				relScript := filepath.Join(dir, fields[0])
				if stat, err := os.Stat(relScript); err == nil && !stat.IsDir() {
					rest := strings.TrimSpace(command[len(fields[0]):])
					var fullCmd string
					if rest != "" {
						fullCmd = fmt.Sprintf("%s %s", relScript, rest)
					} else {
						fullCmd = fmt.Sprintf("%s %s", relScript, suit.Name)
					}
					_ = utils.ExecLogOnly(fullCmd, tailorLogFile)
					continue
				}
			}
			_ = utils.ExecLogOnly(command, tailorLogFile)
		}
		if !isAccessory && ss != nil {
			suffix := ""
			if dryRun {
				suffix = " (simulated)"
			}
			ss.AddStep(fmt.Sprintf("%s[OK]%s Sequence commands completed%s", utils.ColorGreen, utils.ColorReset, suffix))
		}
	}

	return result, nil
}

func checkKernelHeaders(dryRun bool, ss *utils.SplitScreen) {
	if ss != nil {
		ss.SetAction("Checking kernel headers for DKMS...")
		if err := ensureWearHeaders(dryRun); err != nil {
			ss.AddStep(fmt.Sprintf("%s[WARN]%s Kernel headers verification completed with warnings", utils.ColorYellow, utils.ColorReset))
		} else {
			statusMsg := "Kernel headers verified"
			if dryRun {
				statusMsg += " (simulated)"
			}
			ss.AddStep(fmt.Sprintf("%s[OK]%s %s", utils.ColorGreen, utils.ColorReset, statusMsg))
		}
		return
	}

	spHeaders := utils.NewSpinner("Checking kernel headers for DKMS...")
	spHeaders.Start()
	if err := ensureWearHeaders(dryRun); err != nil {
		spHeaders.Warn("Kernel headers verification completed with warnings")
	} else {
		statusMsg := "Kernel headers verified"
		if dryRun {
			statusMsg += " (simulated)"
		}
		spHeaders.Success("%s", statusMsg)
	}
}

// applySysroot applies the sysroot/ or dirs/ filesystem overlay
func applySysroot(dir string, suitName string, dryRun bool, isAccessory bool) {
	ss := utils.GetSplitScreen()
	sysrootPath := filepath.Join(dir, "sysroot")
	if _, err := os.Stat(sysrootPath); os.IsNotExist(err) {
		sysrootPath = filepath.Join(dir, "dirs")
	}
	if _, err := os.Stat(sysrootPath); err == nil {
		if ss != nil {
			if suitName != "" {
				ss.SetAction("%s, applying system configuration (sysroot)...", suitName)
			} else {
				ss.SetAction("Applying system configuration (sysroot)...")
			}
		}
		if dryRun {
			logToFile(fmt.Sprintf("[DRY-RUN] Would rsync sysroot configuration %s -> /", sysrootPath))
			if !isAccessory && ss != nil {
				ss.AddStep(fmt.Sprintf("%s[OK]%s System configuration applied (simulated)", utils.ColorGreen, utils.ColorReset))
			}
		} else {
			cmd := fmt.Sprintf("rsync -aAX %s/ /", sysrootPath)
			err := utils.ExecLogOnly(cmd, tailorLogFile)
			if !isAccessory && ss != nil {
				if err != nil {
					ss.AddStep(fmt.Sprintf("%s[WARN]%s System configuration applied with warnings", utils.ColorYellow, utils.ColorReset))
				} else {
					ss.AddStep(fmt.Sprintf("%s[OK]%s System configuration applied", utils.ColorGreen, utils.ColorReset))
				}
			}
		}
	}
}

// executeFinalizeCommands runs finalization scripts and commands
func executeFinalizeCommands(cmds []string, dir string, suitName string, dryRun bool) {
	if len(cmds) == 0 {
		return
	}
	ss := utils.GetSplitScreen()
	if ss != nil {
		if suitName != "" {
			ss.SetAction("%s, running finalization scripts (%d commands)...", suitName, len(cmds))
		} else {
			ss.SetAction("Running finalization scripts (%d commands)...", len(cmds))
		}
	}
	for idx, command := range cmds {
		fields := strings.Fields(command)
		cmdName := command
		if len(fields) > 0 {
			cmdName = filepath.Base(fields[0])
		}
		if ss != nil {
			if suitName != "" {
				ss.SetAction("%s, [%d/%d] Finalization: %s", suitName, idx+1, len(cmds), cmdName)
			} else {
				ss.SetAction("[%d/%d] Finalization: %s", idx+1, len(cmds), cmdName)
			}
		}
		if dryRun {
			logToFile(fmt.Sprintf("[DRY-RUN] Would run finalization command: %s", command))
			continue
		}
		if len(fields) > 0 {
			relScript := filepath.Join(dir, fields[0])
			if stat, err := os.Stat(relScript); err == nil && !stat.IsDir() {
				rest := strings.TrimSpace(command[len(fields[0]):])
				var fullCmd string
				if rest != "" {
					fullCmd = fmt.Sprintf("%s %s", relScript, rest)
				} else {
					fullCmd = fmt.Sprintf("%s %s", relScript, suitName)
				}
				_ = utils.ExecLogOnly(fullCmd, tailorLogFile)
				continue
			}
		}
		_ = utils.ExecLogOnly(command, tailorLogFile)
	}
	if ss != nil {
		suffix := ""
		if dryRun {
			suffix = " (simulated)"
		}
		ss.AddStep(fmt.Sprintf("%s[OK]%s Finalization completed%s", utils.ColorGreen, utils.ColorReset, suffix))
	}
}

func copySkelToUser(dryRun bool) {
	targetUser := os.Getenv("SUDO_USER")
	var userHome string
	if targetUser != "" {
		userHome = filepath.Join("/home", targetUser)
	} else if u := firstHumanUser(); u != nil {
		targetUser = u.Username
		userHome = u.HomeDir
	}

	if targetUser == "" || targetUser == "root" {
		logToFile("WARNING: unable to determine a non-root target user, skipping /etc/skel sync")
		return
	}

	if dryRun {
		logToFile(fmt.Sprintf("[DRY-RUN] Would sync /etc/skel -> %s", userHome))
		return
	}

	logToFile(fmt.Sprintf("Syncing /etc/skel -> %s", userHome))
	cmd := fmt.Sprintf("rsync -a --no-o --no-g --chown=%s:%s /etc/skel/ %s/", targetUser, targetUser, userHome)
	_ = utils.ExecLogOnly(cmd, tailorLogFile)
}

func waitKeyPress(message string) {
	if !isInteractiveTerminal() {
		return
	}
	if message == "" {
		message = "Press Enter to continue..."
	}
	fmt.Printf("\n%s%s%s%s\n", utils.ColorCyan, utils.ColorBold, message, utils.ColorReset)
	reader := bufio.NewReader(os.Stdin)
	_, _ = reader.ReadString('\n')
}

func rebootSystem() {
	logToFile("Costume requires a reboot. Initiating system restart...")
	waitKeyPress("Press Enter to reset...")
	if err := exec.Command("systemctl", "reboot").Run(); err != nil {
		if err2 := exec.Command("reboot").Run(); err2 != nil {
			_ = exec.Command("shutdown", "-r", "now").Run()
		}
	}
}

func promptConfirm(question string) bool {
	if !isInteractiveTerminal() {
		return false
	}
	cyan := utils.ColorCyan
	reset := utils.ColorReset
	if utils.DisableColors {
		cyan = ""
		reset = ""
	}
	fmt.Printf("\n%s[tailor]%s %s [y/N]: ", cyan, reset, question)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}

func resolveCostumeDir(v2Dir, costumeName string) (string, error) {
	costumeDir := filepath.Join(v2Dir, "costumes", costumeName)
	if _, err := os.Stat(costumeDir); os.IsNotExist(err) {
		if strings.HasPrefix(costumeName, "accessories/") || strings.HasPrefix(costumeName, "costumes/") {
			costumeDir = filepath.Join(v2Dir, costumeName)
		} else {
			accDir := filepath.Join(v2Dir, "accessories", costumeName)
			if _, errAcc := os.Stat(accDir); errAcc == nil {
				costumeDir = accDir
			}
		}
	}
	if _, err := os.Stat(costumeDir); os.IsNotExist(err) {
		return "", fmt.Errorf("costume '%s' not found in %s", costumeName, costumeDir)
	}
	return costumeDir, nil
}

func resolveAccessoryDir(v2Dir, parentDir, accName string) string {
	if strings.HasPrefix(accName, "./") || strings.HasPrefix(accName, "../") {
		return filepath.Join(parentDir, accName)
	}
	if strings.HasPrefix(accName, "accessories/") {
		return filepath.Join(v2Dir, accName)
	}
	return filepath.Join(v2Dir, "accessories", accName)
}

func applyAccessoriesSysroot(v2Dir, parentDir string, accessories []string, dryRun bool) {
	for _, accName := range accessories {
		accDir := resolveAccessoryDir(v2Dir, parentDir, accName)
		accYaml := findYaml(accDir)
		if accYaml == "" {
			continue
		}
		accSuit, err := loadSuit(accYaml)
		if err != nil {
			continue
		}
		if len(accSuit.Accessories) > 0 {
			applyAccessoriesSysroot(v2Dir, accDir, accSuit.Accessories, dryRun)
		}
		applyWearSysroot(accDir, accSuit.Name, dryRun, true)
	}
}

func wearSysrootOnly(costumeName string, linear bool, branch string, dryRun bool, systemIdentity string) error {
	root, err := getWearWardrobeRoot()
	if err != nil {
		utils.LogError("Wardrobe root error: %v", err)
		return err
	}

	if branch != "" {
		if err := Get("", branch); err != nil {
			return fmt.Errorf("failed to get costumes repository on branch '%s': %w", branch, err)
		}
	} else if _, errStat := os.Stat(root); os.IsNotExist(errStat) {
		if stat, errV2 := os.Stat("v2"); errV2 != nil || !stat.IsDir() {
			if err := Get("", ""); err != nil {
				return fmt.Errorf("failed to download costumes repository: %w", err)
			}
		}
	}

	v2Dir, err := getWearWardrobeV2Dir()
	if err != nil {
		utils.LogError("Wardrobe root error: %v", err)
		return err
	}

	costumeDir, err := resolveCostumeDir(v2Dir, costumeName)
	if err != nil {
		return err
	}

	yamlFile := findYaml(costumeDir)
	suit, err := loadSuit(yamlFile)
	if err != nil {
		return err
	}

	isDirectAccessory := strings.HasPrefix(costumeName, "accessories/") || (suit.Name != "" && !strings.Contains(costumeDir, "/costumes/"))

	origin := GetWardrobeOrigin()
	activeBranch := GetWardrobeBranch()

	costumeLabel := fmt.Sprintf("Costume: %s", suit.Name)
	if suit.Release != "" {
		costumeLabel = fmt.Sprintf("Costume: %s (v%s)", suit.Name, suit.Release)
	}
	if isDirectAccessory {
		costumeLabel = fmt.Sprintf("Accessory: %s", suit.Name)
		if suit.Release != "" {
			costumeLabel = fmt.Sprintf("Accessory: %s (v%s)", suit.Name, suit.Release)
		}
	}

	notes := suit.Description
	if notes != "" {
		notes += " - (sysroot & home only)"
	} else {
		notes = "Sysroot & home only"
	}
	if dryRun {
		notes = "[DRY-RUN] " + notes
	}

	headerCfg := utils.SplitScreenConfig{
		Atelier: origin,
		System:  systemIdentity,
		Costume: costumeLabel,
		Branch:  activeBranch,
		Notes:   notes,
	}

	var ss *utils.SplitScreen
	if !linear {
		ss = utils.StartSplitScreenConfig(headerCfg)
		if ss != nil {
			defer ss.Close()
		}
	}
	if ss == nil {
		utils.PrintBannerConfig(headerCfg)
	}

	// Apply accessories sysroot
	if len(suit.Accessories) > 0 {
		if ss != nil {
			ss.SetAction("Applying accessories sysroot (%d items)...", len(suit.Accessories))
		} else {
			utils.PrintSection("👝", fmt.Sprintf("ACCESSORIES (%d items)", len(suit.Accessories)))
		}
		applyAccessoriesSysroot(v2Dir, costumeDir, suit.Accessories, dryRun)
		if ss != nil {
			ss.AddStep(fmt.Sprintf("%s[OK]%s Accessories sysroot applied", utils.ColorGreen, utils.ColorReset))
		}
	}

	// Apply costume sysroot
	if ss != nil {
		costumeActionTag := "Costume"
		if isDirectAccessory {
			costumeActionTag = "Accessory"
		}
		ss.SetAction("%s: %s, applying system configuration (sysroot)...", costumeActionTag, suit.Name)
	} else {
		utils.PrintSubSection("-->", fmt.Sprintf("Applying system configuration (sysroot) for %s...", suit.Name))
	}
	applyWearSysroot(costumeDir, suit.Name, dryRun, false)

	// User environment synchronization
	targetUser := getTargetUsername()
	if targetUser != "" && targetUser != "root" {
		if ss != nil {
			ss.SetAction("Synchronizing user environment (/etc/skel -> /home/%s)...", targetUser)
		} else {
			utils.PrintSubSection("-->", fmt.Sprintf("Synchronizing user environment (/etc/skel -> /home/%s)...", targetUser))
		}
		copyWearSkelToUser(dryRun)
		if ss != nil {
			statusMsg := fmt.Sprintf("User environment synchronized (%s)", targetUser)
			if dryRun {
				statusMsg += " (simulated)"
			}
			ss.AddStep(fmt.Sprintf("%s[OK]%s %s", utils.ColorGreen, utils.ColorReset, statusMsg))
		}
	}

	// Close split screen before printing final summary box
	if ss != nil {
		ss.Close()
		ss = nil
	}

	reportPath, reportErr := writeWearReport(wearReport{
		CostumeName:   suit.Name,
		System:        systemIdentity,
		Installed:     nil,
		Unavailable:   nil,
		FailedInstall: nil,
	})

	costumeSummaryVal := suit.Name
	if dryRun {
		costumeSummaryVal = fmt.Sprintf("%s [DRY-RUN]", suit.Name)
	}
	summaryRows := [][2]string{
		{"Costume / Item", costumeSummaryVal},
	}
	if origin != "" {
		atelierVal := origin
		if activeBranch != "" && activeBranch != "main" && activeBranch != "master" {
			atelierVal = fmt.Sprintf("%s (%s)", origin, activeBranch)
		}
		summaryRows = append(summaryRows, [2]string{"Atelier", atelierVal})
	}
	summaryRows = append(summaryRows,
		[2]string{"Mode", "Sysroot & home configuration only"},
		[2]string{"System configuration", "Applied (sysroot overlay)"},
	)
	if targetUser != "" && targetUser != "root" {
		userStatus := fmt.Sprintf("Synchronized (/home/%s)", targetUser)
		if dryRun {
			userStatus += " (simulated)"
		}
		summaryRows = append(summaryRows, [2]string{"User environment", userStatus})
	}
	if reportErr == nil {
		summaryRows = append(summaryRows, [2]string{"Detailed report", reportPath})
	}
	if !dryRun {
		summaryRows = append(summaryRows, [2]string{"System log", tailorLogFile})
	}

	summaryTitle := "✨ WEAR COMPLETED (SYSROOT ONLY)!"
	if dryRun {
		summaryTitle = "✨ WEAR COMPLETED (SIMULATION - SYSROOT ONLY)!"
	}
	utils.PrintSummaryBox(summaryTitle, summaryRows)
	return nil
}
