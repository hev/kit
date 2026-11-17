import type { APIRoute } from "astro";
import { llmsFull } from "../lib/docs-export";

export const prerender = true;
export const GET: APIRoute = () => llmsFull();
