import csv
from collections import defaultdict
from pathlib import Path
import statistics

import matplotlib.pyplot as plt


ROOT = Path(__file__).resolve().parents[1]
RESULTS = ROOT / "results"
OUTPUT = ROOT / "docs" / "images"

INPUTS = [
    RESULTS / "policy_evaluation_64mib.csv",
    RESULTS / "policy_evaluation_128mib.csv",
]

POLICIES = [
    "static",
    "naive",
    "cost-aware",
]


def load_rows():
    rows = []

    for path in INPUTS:
        with path.open(newline="", encoding="utf-8") as handle:
            reader = csv.DictReader(handle)

            for row in reader:
                rows.append(
                    {
                        "policy": row["policy"],
                        "state_mb": int(row["state_mb"]),
                        "work": int(row["work"]),
                        "completion_seconds": float(
                            row["completion_seconds"]
                        ),
                        "transition_count": int(
                            row["transition_count"]
                        ),
                    }
                )

    return rows


def median_completion(rows):
    grouped = defaultdict(list)

    for row in rows:
        key = (
            row["state_mb"],
            row["work"],
            row["policy"],
        )

        grouped[key].append(
            row["completion_seconds"]
        )

    return {
        key: statistics.median(values)
        for key, values in grouped.items()
    }


def transition_frequency(rows):
    grouped = defaultdict(list)

    for row in rows:
        if row["policy"] != "cost-aware":
            continue

        key = (
            row["state_mb"],
            row["work"],
        )

        grouped[key].append(
            row["transition_count"] > 0
        )

    return {
        key: sum(values) / len(values)
        for key, values in grouped.items()
    }


def plot_completion(rows):
    medians = median_completion(rows)

    for state_mb in (64, 128):
        works = sorted(
            {
                row["work"]
                for row in rows
                if row["state_mb"] == state_mb
            }
        )

        x = list(range(len(works)))
        width = 0.24

        fig, ax = plt.subplots(
            figsize=(8.5, 5.0)
        )

        for index, policy in enumerate(POLICIES):
            values = [
                medians[
                    (
                        state_mb,
                        work,
                        policy,
                    )
                ]
                for work in works
            ]

            offsets = [
                value +
                (index - 1) * width
                for value in x
            ]

            bars = ax.bar(
                offsets,
                values,
                width,
                label=policy,
            )

            ax.bar_label(
                bars,
                fmt="%.1f",
                padding=3,
                fontsize=8,
            )

        labels = [
            (
                f"{work // 1_000_000}M"
                if work >= 1_000_000
                else f"{work // 1000}k"
            )
            for work in works
        ]

        ax.set_xticks(x)
        ax.set_xticklabels(labels)

        ax.set_xlabel("Work units")
        ax.set_ylabel("Median completion time (s)")

        ax.set_title(
            f"Forge policy performance — {state_mb} MiB state"
        )

        ax.legend(
            title="Policy"
        )

        ax.grid(
            axis="y",
            alpha=0.25,
        )

        fig.tight_layout()

        output = (
            OUTPUT /
            f"policy_completion_{state_mb}mib.png"
        )

        fig.savefig(
            output,
            dpi=180,
        )

        plt.close(fig)

        print(f"wrote {output}")


def plot_transition_frequency(rows):
    frequencies = transition_frequency(rows)

    labels = []
    values = []

    for state_mb in (64, 128):
        works = sorted(
            work
            for candidate_state, work
            in frequencies
            if candidate_state == state_mb
        )

        for work in works:
            work_label = (
                f"{work // 1_000_000}M"
                if work >= 1_000_000
                else f"{work // 1000}k"
            )

            labels.append(
                f"{state_mb} MiB\n{work_label}"
            )

            values.append(
                frequencies[
                    (
                        state_mb,
                        work,
                    )
                ] * 100
            )

    fig, ax = plt.subplots(
        figsize=(8.5, 5.0)
    )

    bars = ax.bar(
        range(len(labels)),
        values,
    )

    ax.bar_label(
        bars,
        fmt="%.0f%%",
        padding=3,
    )

    ax.set_xticks(
        range(len(labels))
    )

    ax.set_xticklabels(
        labels
    )

    ax.set_ylim(
        0,
        110,
    )

    ax.set_ylabel(
        "Trials that transitioned (%)"
    )

    ax.set_xlabel(
        "State size and workload"
    )

    ax.set_title(
        "Forge cost-aware transition decisions"
    )

    ax.grid(
        axis="y",
        alpha=0.25,
    )

    fig.tight_layout()

    output = (
        OUTPUT /
        "cost_aware_transition_frequency.png"
    )

    fig.savefig(
        output,
        dpi=180,
    )

    plt.close(fig)

    print(f"wrote {output}")


def main():
    OUTPUT.mkdir(
        parents=True,
        exist_ok=True,
    )

    rows = load_rows()

    print(
        f"loaded {len(rows)} observations"
    )

    plot_completion(rows)
    plot_transition_frequency(rows)


if __name__ == "__main__":
    main()