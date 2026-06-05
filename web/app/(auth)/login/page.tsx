"use client";

import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import {
  Form,
  TextInput,
  PasswordInput,
  Button,
  InlineNotification,
  Tile,
  Stack,
} from "@carbon/react";
import { useRouter } from "next/navigation";
import { useState } from "react";

const loginSchema = z.object({
  email: z.string().email("Valid email address required"),
  password: z.string().min(1, "Password is required"),
});

type LoginFormValues = z.infer<typeof loginSchema>;

/**
 * Console login page.
 * Calls the /api/auth/login BFF route which exchanges credentials for a session cookie.
 */
export default function LoginPage() {
  const router = useRouter();
  const [apiError, setApiError] = useState<string | null>(null);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<LoginFormValues>({
    resolver: zodResolver(loginSchema),
  });

  async function onSubmit(values: LoginFormValues) {
    setApiError(null);

    try {
      const res = await fetch("/api/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(values),
      });

      if (res.ok) {
        router.push("/dashboard");
        router.refresh();
      } else {
        const data = (await res.json()) as { error?: string };
        setApiError(data.error ?? "Login failed. Please try again.");
      }
    } catch {
      setApiError("Network error. Please check your connection.");
    }
  }

  return (
    <Tile style={{ width: 400, padding: "2rem" }}>
      <Stack gap={6}>
        <div>
          <h1 style={{ fontSize: "1.5rem", fontWeight: 600, marginBottom: "0.25rem" }}>
            {process.env.NEXT_PUBLIC_APP_NAME ?? "Pangreksa Console"}
          </h1>
          <p style={{ color: "#6f6f6f", fontSize: "0.875rem" }}>
            Sign in to your account
          </p>
        </div>

        {apiError !== null && (
          <InlineNotification
            kind="error"
            title="Sign in failed"
            subtitle={apiError}
            lowContrast
            hideCloseButton
          />
        )}

        <Form onSubmit={handleSubmit(onSubmit)}>
          <Stack gap={5}>
            <TextInput
              id="email"
              labelText="Email address"
              type="email"
              placeholder="admin@example.com"
              autoComplete="email"
              {...register("email")}
              invalid={!!errors.email}
              invalidText={errors.email?.message}
            />

            <PasswordInput
              id="password"
              labelText="Password"
              placeholder="••••••••"
              autoComplete="current-password"
              {...register("password")}
              invalid={!!errors.password}
              invalidText={errors.password?.message}
            />

            <Button
              type="submit"
              disabled={isSubmitting}
              style={{ width: "100%", maxWidth: "none" }}
            >
              {isSubmitting ? "Signing in…" : "Sign in"}
            </Button>
          </Stack>
        </Form>
      </Stack>
    </Tile>
  );
}
