import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import Settings from "./Settings";

function settingsWS(overrides = {}) {
  return {
    connected: true,
    call: vi.fn(async (method) => {
      if (method === "config.providers") {
        return {
          providers: [
            {
              id: "anthropic",
              name: "Anthropic",
              envVar: "ANTHROPIC_API_KEY",
              defaultModel: "claude-sonnet",
              models: ["claude-sonnet"],
              needsKey: true,
            },
            {
              id: "openai",
              name: "OpenAI",
              envVar: "OPENAI_API_KEY",
              defaultModel: "openai/gpt-5.4",
              models: ["openai/gpt-5.4"],
              needsKey: true,
            },
          ],
        };
      }
      if (method === "config.get") {
        return {
          provider: "anthropic",
          model: "claude-sonnet",
          hasKey: false,
          setupComplete: true,
          defaultBudgetMaxCostUsd: 1.5,
          defaultBudgetMaxTokens: 6000,
          ...overrides.config,
        };
      }
      if (method === "status.get") {
        return {
          browser_route: "camoufox",
          browser_route_source: "runtime",
          browser_window: "hidden",
          gateway_running: true,
          kernel_headless: false,
          kernel_running: true,
          openclaw_profile_configured: true,
          ...overrides.status,
        };
      }
      if (method === "config.set") {
        return overrides.configSet || {};
      }
      return {};
    }),
    notify: vi.fn(),
  };
}

describe("Settings page", () => {
  it("shows route, mode, and gateway status", async () => {
    const ws = settingsWS();

    render(<Settings ws={ws} />);

    expect(
      await screen.findByText("Route: CAMOUFOX (runtime) · GUI"),
    ).toBeInTheDocument();
    expect(screen.getByText("Window: HIDDEN")).toBeInTheDocument();
    expect(screen.getByText("Gateway: RUNNING")).toBeInTheDocument();
  });

  it("saves provider credentials", async () => {
    const ws = settingsWS();

    render(<Settings ws={ws} />);

    const keyInput = await screen.findByPlaceholderText("sk-...");
    fireEvent.change(keyInput, { target: { value: "sk-secret" } });
    fireEvent.click(screen.getByText("Save Provider"));

    await waitFor(() => {
      expect(ws.call).toHaveBeenCalledWith("config.set", {
        provider: "anthropic",
        model: "claude-sonnet",
        apiKey: "sk-secret",
      });
    });
    await waitFor(() => {
      expect(keyInput).toHaveValue("");
    });
    expect(
      screen.getByText(
        "A key is already stored locally. Leave this blank to keep it.",
      ),
    ).toBeInTheDocument();
  });

  it("does not mark an empty provider key as stored", async () => {
    const ws = settingsWS();

    render(<Settings ws={ws} />);

    expect(await screen.findByText("No key stored yet.")).toBeInTheDocument();
    fireEvent.click(screen.getByText("Save Provider"));

    await waitFor(() => {
      expect(ws.call).toHaveBeenCalledWith("config.set", {
        provider: "anthropic",
        model: "claude-sonnet",
        apiKey: "",
      });
    });
    expect(screen.getByText("No key stored yet.")).toBeInTheDocument();
  });

  it("does not carry a stored key across provider changes", async () => {
    const ws = settingsWS({
      config: { hasKey: true },
      configSet: {
        status: "ok",
        config: {
          provider: "openai",
          model: "openai/gpt-5.4",
          hasKey: false,
          setupComplete: false,
        },
      },
    });

    render(<Settings ws={ws} />);

    expect(
      await screen.findByText(
        "A key is already stored locally. Leave this blank to keep it.",
      ),
    ).toBeInTheDocument();
    fireEvent.change(screen.getAllByRole("combobox")[0], {
      target: { value: "openai" },
    });
    expect(screen.getByText("No key stored yet.")).toBeInTheDocument();
    fireEvent.click(screen.getByText("Save Provider"));

    await waitFor(() => {
      expect(ws.call).toHaveBeenCalledWith("config.set", {
        provider: "openai",
        model: "openai/gpt-5.4",
        apiKey: "",
      });
    });
    expect(screen.getByText("No key stored yet.")).toBeInTheDocument();
  });

  it("saves default budget values", async () => {
    const ws = settingsWS();

    render(<Settings ws={ws} />);

    const costInput = await screen.findByDisplayValue("1.5");
    const tokenInput = screen.getByDisplayValue("6000");
    fireEvent.change(costInput, { target: { value: "2.25" } });
    fireEvent.change(tokenInput, { target: { value: "9000" } });
    fireEvent.click(screen.getByText("Save Defaults"));

    await waitFor(() => {
      expect(ws.call).toHaveBeenCalledWith("config.set", {
        defaultBudgetMaxCostUsd: 2.25,
        defaultBudgetMaxTokens: 9000,
      });
    });
  });
});
