import type { APIRoute } from "astro";
import { llmsIndex } from "../lib/docs-export";

export const prerender = true;
export const GET: APIRoute = () => llmsIndex();
