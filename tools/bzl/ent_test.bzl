"""Analysis test for ent.bzl: the generation action is declared with its flags file and package check."""

load("@bazel_skylib//lib:unittest.bzl", "analysistest", "asserts")

def _ent_gen_action_test_impl(ctx):
    env = analysistest.begin(ctx)
    actions = [a for a in analysistest.target_actions(env) if a.mnemonic == "EntGen"]
    asserts.equals(env, 1, len(actions))
    argv = actions[0].argv
    asserts.true(env, "-expect" in argv, "EntGen must pass -expect so a stale packages.bzl fails the build")
    asserts.true(env, "-flags" in argv, "EntGen must read the flags file (ent_flags.txt)")
    asserts.true(env, [i for i in actions[0].inputs.to_list() if i.basename == "ent_flags.txt"], "ent_flags.txt must be an action input")
    return analysistest.end(env)

ent_gen_action_test = analysistest.make(_ent_gen_action_test_impl)
