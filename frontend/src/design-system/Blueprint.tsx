/**
 * Moldura "blueprint" do sistema Industry: canto reto, borda hairline e as
 * quatro marcas de registro "+". O readme do sistema exige que nenhum
 * elemento emoldurado perca as marcas.
 */

/** As quatro marcas de registro. Use dentro de qualquer elemento `.blueprint`. */
export function BlueprintCorners() {
  return (
    <>
      <i className="corner tl" />
      <i className="corner tr" />
      <i className="corner bl" />
      <i className="corner br" />
    </>
  );
}
