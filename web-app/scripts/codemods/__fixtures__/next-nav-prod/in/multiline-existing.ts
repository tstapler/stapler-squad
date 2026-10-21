import { enumParam } from "@/lib/nav";
import {
  usePathname,
  useRouter as useNextRouter,
} from "next/navigation";

export const used = [enumParam, usePathname, useNextRouter];
