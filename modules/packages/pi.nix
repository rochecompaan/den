{ ... }:
{
  perSystem = { self', ... }: {
    packages.pi = self'.lib.mkPi { };
  };
}
